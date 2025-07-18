package chunkenc

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"hash"
	"io"
	"strings"
	"sync"

	"github.com/pkg/errors"
	"github.com/prometheus/otlptranslator"
	"github.com/prometheus/prometheus/model/labels"

	"github.com/grafana/loki/v3/pkg/compression"
	"github.com/grafana/loki/v3/pkg/util"
	"github.com/grafana/loki/v3/pkg/util/labelpool"
)

var errSymbolizerReadOnly = errors.New("writes not allowed when symbolizer is in read-only mode")

// symbol holds reference to a label name and value pair
type symbol struct {
	Name, Value uint32
}

type symbols []symbol

// symbolizer holds a collection of label names and values and assign symbols to them.
// symbols are actually index numbers assigned based on when the entry is seen for the first time.
type symbolizer struct {
	mtx            sync.RWMutex
	symbolsMap     map[string]uint32 // symbolsMap is only used on the write path for deduping symbol writes
	labels         []string
	size           int
	compressedSize int
	readOnly       bool
	// Runtime-only map to track which symbols are label names and have been normalized
	normalizedNames map[uint32]string
}

func newSymbolizer() *symbolizer {
	return &symbolizer{
		symbolsMap:      map[string]uint32{},
		normalizedNames: map[uint32]string{},
	}
}

// Add adds new labels pairs to the collection and returns back a symbol for each existing and new label pair
func (s *symbolizer) Add(lbls labels.Labels) (symbols, error) {
	if s.readOnly {
		return nil, errSymbolizerReadOnly
	}
	if lbls.IsEmpty() {
		return nil, nil
	}

	syms := make([]symbol, 0, lbls.Len())

	lbls.Range(func(label labels.Label) {
		syms = append(syms, symbol{
			Name:  s.add(label.Name),
			Value: s.add(label.Value),
		})
	})

	return syms, nil
}

func (s *symbolizer) add(lbl string) uint32 {
	s.mtx.RLock()
	idx, ok := s.symbolsMap[lbl]
	s.mtx.RUnlock()

	if ok {
		return idx
	}

	s.mtx.Lock()
	defer s.mtx.Unlock()

	idx, ok = s.symbolsMap[lbl]
	if !ok {
		lbl = strings.Clone(lbl)
		idx = uint32(len(s.labels))
		s.symbolsMap[lbl] = idx
		s.labels = append(s.labels, lbl)
		s.size += len(lbl)
	}

	return idx
}

// Lookup coverts and returns labels pairs for the given symbols
func (s *symbolizer) Lookup(syms symbols, buf *labels.ScratchBuilder) labels.Labels {
	if len(syms) == 0 {
		return labels.EmptyLabels()
	}

	if buf == nil {
		buf = labelpool.Get()
		defer labelpool.Put(buf)
	} else {
		buf.Reset()
	}

	labelNamer := otlptranslator.LabelNamer{}
	for _, symbol := range syms {
		// First check if we have a normalized name for this symbol
		s.mtx.RLock()
		normalized, exists := s.normalizedNames[symbol.Name]
		s.mtx.RUnlock()

		var name string
		if exists {
			name = normalized
		} else {
			// If we haven't seen this name before, look it up and normalize it
			name = s.lookup(symbol.Name)
			normalized := labelNamer.Build(name)
			s.mtx.Lock()
			s.normalizedNames[symbol.Name] = normalized
			s.mtx.Unlock()
			name = normalized
		}

		buf.Add(name, s.lookup(symbol.Value))
	}

	return buf.Labels()
}

func (s *symbolizer) lookup(idx uint32) string {
	// take a read lock only if we will be getting new entries
	if !s.readOnly {
		s.mtx.RLock()
		defer s.mtx.RUnlock()
	}

	if idx >= uint32(len(s.labels)) {
		return ""
	}

	return s.labels[idx]
}

// UncompressedSize returns the number of bytes taken up by deduped string labels
func (s *symbolizer) UncompressedSize() int {
	s.mtx.RLock()
	defer s.mtx.RUnlock()

	return s.size
}

// CompressedSize returns the number of bytes that were taken by serialized symbolizer which means it is set only when deserialized from serialized bytes
func (s *symbolizer) CompressedSize() int {
	return s.compressedSize
}

// DecompressedSize returns the number of bytes we would have gotten after decompressing serialized bytes.
// It returns non-zero value only if we actually loaded it from serialized bytes by looking at value returned by CompressedSize() because
// we only want to consider data being decompressed if it actually was done.
func (s *symbolizer) DecompressedSize() int {
	if s.CompressedSize() == 0 {
		return 0
	}
	return s.CheckpointSize()
}

// CheckpointSize returns the number of bytes it would take for writing labels as checkpoint
func (s *symbolizer) CheckpointSize() int {
	s.mtx.RLock()
	defer s.mtx.RUnlock()

	size := binary.MaxVarintLen32                 // number of labels
	size += len(s.labels) * binary.MaxVarintLen32 // length of each label
	size += s.size                                // number of bytes occupied by labels

	return size
}

// SerializeTo serializes all the labels and writes to the writer in compressed format.
// It returns back the number of bytes written and a checksum of the data written.
func (s *symbolizer) SerializeTo(w io.Writer, pool compression.WriterPool) (int, []byte, error) {
	crc32Hash := crc32HashPool.Get().(hash.Hash32)
	defer crc32HashPool.Put(crc32Hash)

	eb := EncodeBufferPool.Get().(*encbuf)
	defer EncodeBufferPool.Put(eb)

	writtenBytes := 0
	crc32Hash.Reset()
	eb.reset()

	s.mtx.RLock()
	defer s.mtx.RUnlock()

	// write the number of labels without compressing it to make it easier to read the number of labels
	eb.putUvarint(len(s.labels))

	_, err := crc32Hash.Write(eb.get())
	if err != nil {
		return 0, nil, errors.Wrap(err, "write num of labels of structured metadata to crc32hash")
	}
	n, err := w.Write(eb.get())
	if err != nil {
		return 0, nil, errors.Wrap(err, "write num of labels of structured metadata to writer")
	}
	writtenBytes += n

	inBuf := serializeBytesBufferPool.Get().(*bytes.Buffer)
	defer func() {
		inBuf.Reset()
		serializeBytesBufferPool.Put(inBuf)
	}()
	outBuf := &bytes.Buffer{}

	encBuf := make([]byte, binary.MaxVarintLen64)
	// write the labels
	for _, label := range s.labels {
		// write the length of the label
		n := binary.PutUvarint(encBuf, uint64(len(label)))
		inBuf.Write(encBuf[:n])

		// write the label
		inBuf.WriteString(label)
	}

	// compress the labels block
	compressedWriter := pool.GetWriter(outBuf)
	defer pool.PutWriter(compressedWriter)

	if _, err := compressedWriter.Write(inBuf.Bytes()); err != nil {
		return 0, nil, errors.Wrap(err, "appending entry")
	}
	if err := compressedWriter.Close(); err != nil {
		return 0, nil, errors.Wrap(err, "flushing pending compress buffer")
	}

	b := outBuf.Bytes()

	// hash the labels block
	_, err = crc32Hash.Write(b)
	if err != nil {
		return writtenBytes, nil, errors.Wrap(err, "build structured metadata hash")
	}

	// write the labels block to writer
	n, err = w.Write(b)
	if err != nil {
		return writtenBytes, nil, errors.Wrap(err, "write structured metadata block")
	}
	writtenBytes += n
	return writtenBytes, crc32Hash.Sum(nil), nil
}

// CheckpointTo writes all the labels to the writer.
// It returns back the number of bytes written and a checksum of the data written.
func (s *symbolizer) CheckpointTo(w io.Writer) (int, []byte, error) {
	crcHash := crc32HashPool.Get().(hash.Hash32)
	defer crc32HashPool.Put(crcHash)

	eb := EncodeBufferPool.Get().(*encbuf)
	defer EncodeBufferPool.Put(eb)

	writtenBytes := 0
	crcHash.Reset()
	eb.reset()

	s.mtx.RLock()
	defer s.mtx.RUnlock()

	// write the number of labels
	eb.putUvarint(len(s.labels))
	n, err := w.Write(eb.get())
	if err != nil {
		return 0, nil, err
	}

	_, err = crcHash.Write(eb.get())
	if err != nil {
		return writtenBytes, nil, err
	}

	eb.reset()
	writtenBytes += n

	for _, label := range s.labels {
		// write label length
		eb.putUvarint(len(label))
		n, err := w.Write(eb.get())
		if err != nil {
			return writtenBytes, nil, err
		}

		_, err = crcHash.Write(eb.get())
		if err != nil {
			return writtenBytes, nil, err
		}

		eb.reset()
		writtenBytes += n

		// write the label

		_, err = crcHash.Write(util.YoloBuf(label))
		if err != nil {
			return writtenBytes, nil, err
		}

		n, err = io.WriteString(w, label)
		if err != nil {
			return writtenBytes, nil, err
		}
		writtenBytes += n
	}

	return writtenBytes, crcHash.Sum(nil), nil
}

// symbolizerFromCheckpoint builds symbolizer from the bytes generated during a checkpoint.
func symbolizerFromCheckpoint(b []byte) *symbolizer {
	if len(b) == 0 {
		return newSymbolizer()
	}

	db := decbuf{b: b}
	numLabels := db.uvarint()
	s := symbolizer{
		symbolsMap: make(map[string]uint32, numLabels),
		labels:     make([]string, 0, numLabels),
		// Labels are key-value pairs, preallocate to half the number to store just the keys,
		// likely less memory than the exponential growth Go will do.
		normalizedNames: make(map[uint32]string, numLabels/2),
	}

	for i := 0; i < numLabels; i++ {
		label := string(db.bytes(db.uvarint()))
		s.labels = append(s.labels, label)
		s.symbolsMap[label] = uint32(i)
		s.size += len(label)
	}

	return &s
}

// symbolizerFromEnc builds symbolizer from the bytes generated during serialization.
// It sets the symbolizer to read-only mode because deserialization is usually done using data from a flushed chunk.
func symbolizerFromEnc(b []byte, pool compression.ReaderPool) (*symbolizer, error) {
	db := decbuf{b: b}
	numLabels := db.uvarint()

	b = db.b

	reader, err := pool.GetReader(bytes.NewBuffer(b))
	if err != nil {
		return nil, err
	}
	defer pool.PutReader(reader)

	s := symbolizer{
		labels: make([]string, 0, numLabels),
		// Same as symbolizerFromCheckpoint
		normalizedNames: make(map[uint32]string, numLabels/2),
		compressedSize:  len(b),
		readOnly:        true,
	}

	var (
		readBuf      [10]byte // Enough bytes to store one varint.
		readBufValid int      // How many bytes are left in readBuf from previous read.
		buf          []byte
	)

	for i := 0; i < numLabels; i++ {
		var lWidth, labelSize, lastAttempt int
		for lWidth == 0 { // Read until both varints have enough bytes.
			n, err := reader.Read(readBuf[readBufValid:])
			readBufValid += n
			if err != nil {
				if err != io.EOF {
					return nil, err
				}
				if readBufValid == 0 { // Got EOF and no data in the buffer.
					return nil, fmt.Errorf("got unexpected EOF")
				}
				if readBufValid == lastAttempt { // Got EOF and could not parse same data last time.
					return nil, fmt.Errorf("invalid structured metadata block in chunk")
				}
			}
			var l uint64
			l, lWidth = binary.Uvarint(readBuf[:readBufValid])
			labelSize = int(l)
			lastAttempt = readBufValid
		}

		// If the buffer is not yet initialize or too small, we get a new one.
		if buf == nil || labelSize > cap(buf) {
			// in case of a replacement we replace back the buffer in the pool
			if buf != nil {
				BytesBufferPool.Put(buf)
			}
			buf = BytesBufferPool.Get(labelSize).([]byte)
			if labelSize > cap(buf) {
				return nil, fmt.Errorf("could not get a label buffer of size %d, actual %d", labelSize, cap(buf))
			}
		}
		buf = buf[:labelSize]
		// Take however many bytes are left in the read buffer.
		n := copy(buf, readBuf[lWidth:readBufValid])
		// Shift down what is still left in the fixed-size read buffer, if any.
		readBufValid = copy(readBuf[:], readBuf[lWidth+n:readBufValid])

		// Then process reading the line.
		for n < labelSize {
			r, err := reader.Read(buf[n:labelSize])
			n += r
			if err != nil {
				// We might get EOF after reading enough bytes to fill the buffer, which is OK.
				// EOF and zero bytes read when the buffer isn't full is an error.
				if err == io.EOF && r != 0 {
					continue
				}
				return nil, err
			}
		}
		label := string(buf)
		s.labels = append(s.labels, label)
		s.size += len(buf)
	}

	return &s, nil
}
