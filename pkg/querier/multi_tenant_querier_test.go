package querier

import (
	"context"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"testing"
	"time"
	"unicode"

	"github.com/axiomhq/hyperloglog"
	"github.com/go-kit/log"
	"github.com/grafana/dskit/user"
	"github.com/prometheus/prometheus/model/labels"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/grafana/loki/v3/pkg/iter"
	"github.com/grafana/loki/v3/pkg/logproto"
	"github.com/grafana/loki/v3/pkg/logql"
	"github.com/grafana/loki/v3/pkg/logql/syntax"
	"github.com/grafana/loki/v3/pkg/querier/plan"
)

func TestMultiTenantQuerier_SelectLogs(t *testing.T) {
	for _, tc := range []struct {
		desc      string
		orgID     string
		selector  string
		expLabels []string
		expLines  []string
	}{
		{
			"two tenants",
			"1|2",
			`{type="test"}`,
			[]string{
				`{__tenant_id__="1", type="test"}`,
				`{__tenant_id__="1", type="test"}`,
				`{__tenant_id__="2", type="test"}`,
				`{__tenant_id__="2", type="test"}`,
			},
			[]string{"line 1", "line 2", "line 1", "line 2"},
		},
		{
			"two tenants with selector",
			"1|2",
			`{type="test", __tenant_id__="1"}`,
			[]string{
				`{__tenant_id__="1", type="test"}`,
				`{__tenant_id__="1", type="test"}`,
			},
			[]string{"line 1", "line 2", "line 1", "line 2"},
		},
		{
			"two tenants with selector and pipeline filter",
			"1|2",
			`{type="test", __tenant_id__!="2"} | logfmt | some_lable="foobar"`,
			[]string{
				`{__tenant_id__="1", type="test"}`,
				`{__tenant_id__="1", type="test"}`,
			},
			[]string{"line 1", "line 2", "line 1", "line 2"},
		},
		{
			"one tenant",
			"1",
			`{type="test"}`,
			[]string{
				`{type="test"}`,
				`{type="test"}`,
			},
			[]string{"line 1", "line 2"},
		},
	} {
		t.Run(tc.desc, func(t *testing.T) {
			querier := newQuerierMock()
			querier.On("SelectLogs", mock.Anything, mock.Anything).Return(func() iter.EntryIterator { return mockStreamIterator(1, 2) }, nil)

			multiTenantQuerier := NewMultiTenantQuerier(querier, log.NewNopLogger())

			ctx := user.InjectOrgID(context.Background(), tc.orgID)
			params := logql.SelectLogParams{QueryRequest: &logproto.QueryRequest{
				Selector:  tc.selector,
				Direction: logproto.BACKWARD,
				Limit:     0,
				Shards:    nil,
				Start:     time.Unix(0, 1),
				End:       time.Unix(0, time.Now().UnixNano()),
				Plan: &plan.QueryPlan{
					AST: syntax.MustParseExpr(tc.selector),
				},
			}}
			iter, err := multiTenantQuerier.SelectLogs(ctx, params)
			require.NoError(t, err)

			entriesCount := 0
			for iter.Next() {
				require.Equal(t, tc.expLabels[entriesCount], iter.Labels())
				require.Equal(t, tc.expLines[entriesCount], iter.At().Line)
				entriesCount++
			}
			require.Equalf(t, len(tc.expLabels), entriesCount, "Expected %d entries but got %d", len(tc.expLabels), entriesCount)
		})
	}
}

func TestMultiTenantQuerier_SelectSamples(t *testing.T) {
	for _, tc := range []struct {
		desc      string
		orgID     string
		selector  string
		expLabels []string
	}{
		{
			"two tenants",
			"1|2",
			`count_over_time({foo="bar"}[1m]) > 10`,
			[]string{
				`{__tenant_id__="1", app="foo"}`,
				`{__tenant_id__="2", app="foo"}`,
				`{__tenant_id__="2", app="bar"}`,
				`{__tenant_id__="1", app="bar"}`,
				`{__tenant_id__="1", app="foo"}`,
				`{__tenant_id__="2", app="foo"}`,
				`{__tenant_id__="2", app="bar"}`,
				`{__tenant_id__="1", app="bar"}`,
			},
		},
		{
			"two tenants with selector",
			"1|2",
			`count_over_time({foo="bar", __tenant_id__="1"}[1m]) > 10`,
			[]string{
				`{__tenant_id__="1", app="foo"}`,
				`{__tenant_id__="1", app="bar"}`,
				`{__tenant_id__="1", app="foo"}`,
				`{__tenant_id__="1", app="bar"}`,
			},
		},
		{
			"one tenant",
			"1",
			`count_over_time({foo="bar"}[1m]) > 10`,
			[]string{
				`{app="foo"}`,
				`{app="bar"}`,
				`{app="foo"}`,
				`{app="bar"}`,
			},
		},
	} {
		t.Run(tc.desc, func(t *testing.T) {
			querier := newQuerierMock()
			querier.On("SelectSamples", mock.Anything, mock.Anything).Return(func() iter.SampleIterator { return newSampleIterator() }, nil)

			multiTenantQuerier := NewMultiTenantQuerier(querier, log.NewNopLogger())

			ctx := user.InjectOrgID(context.Background(), tc.orgID)
			params := logql.SelectSampleParams{SampleQueryRequest: &logproto.SampleQueryRequest{
				Selector: tc.selector,
				Plan: &plan.QueryPlan{
					AST: syntax.MustParseExpr(tc.selector),
				},
			}}
			iter, err := multiTenantQuerier.SelectSamples(ctx, params)
			require.NoError(t, err)

			received := make([]string, 0, len(tc.expLabels))
			for iter.Next() {
				received = append(received, iter.Labels())
			}
			require.ElementsMatch(t, tc.expLabels, received)
		})
	}
}

func TestMultiTenantQuerier_TenantFilter(t *testing.T) {
	for _, tc := range []struct {
		selector string
		expected string
	}{
		{
			`count_over_time({foo="bar", __tenant_id__="1"}[1m]) > 10`,
			`(count_over_time({foo="bar"}[1m]) > 10)`,
		},
		{
			`topk(2, count_over_time({app="foo", __tenant_id__="1"}[3m]))`,
			`topk(2, count_over_time({app="foo"}[3m]))`,
		},
	} {
		t.Run(tc.selector, func(t *testing.T) {
			params := logql.SelectSampleParams{SampleQueryRequest: &logproto.SampleQueryRequest{
				Selector: tc.selector,
				Plan: &plan.QueryPlan{
					AST: syntax.MustParseExpr(tc.selector),
				},
			}}
			_, updatedSelector, err := removeTenantSelector(params, []string{})
			require.NoError(t, err)
			require.Equal(t, removeWhiteSpace(tc.expected), removeWhiteSpace(updatedSelector.String()))
		})
	}
}

var samples = []logproto.Sample{
	{Timestamp: time.Unix(2, 0).UnixNano(), Hash: 1, Value: 1.},
	{Timestamp: time.Unix(5, 0).UnixNano(), Hash: 2, Value: 1.},
}

var (
	labelFoo, _ = syntax.ParseLabels("{app=\"foo\"}")
	labelBar, _ = syntax.ParseLabels("{app=\"bar\"}")
)

func newSampleIterator() iter.SampleIterator {
	return iter.NewSortSampleIterator([]iter.SampleIterator{
		iter.NewSeriesIterator(logproto.Series{
			Labels:     labelFoo.String(),
			Samples:    samples,
			StreamHash: labels.StableHash(labelFoo),
		}),
		iter.NewSeriesIterator(logproto.Series{
			Labels:     labelBar.String(),
			Samples:    samples,
			StreamHash: labels.StableHash(labelBar),
		}),
	})
}

func BenchmarkTenantEntryIteratorLabels(b *testing.B) {
	it := newMockEntryIterator(12)
	tenantIter := NewTenantEntryIterator(it, "tenant_1")

	b.ResetTimer()
	b.ReportAllocs()

	for n := 0; n < b.N; n++ {
		tenantIter.Labels()
	}
}

type mockEntryIterator struct {
	labels string
}

func newMockEntryIterator(numLabels int) mockEntryIterator {
	builder := labels.NewBuilder(labels.EmptyLabels())
	for i := 1; i <= numLabels; i++ {
		builder.Set(fmt.Sprintf("label_%d", i), strconv.Itoa(i))
	}
	return mockEntryIterator{labels: builder.Labels().String()}
}

func (it mockEntryIterator) Labels() string {
	return it.labels
}

func (it mockEntryIterator) At() logproto.Entry {
	return logproto.Entry{}
}

func (it mockEntryIterator) Next() bool {
	return true
}

func (it mockEntryIterator) StreamHash() uint64 {
	return 0
}

func (it mockEntryIterator) Err() error {
	return nil
}

func (it mockEntryIterator) Close() error {
	return nil
}

func TestMultiTenantQuerier_Label(t *testing.T) {
	start := time.Unix(0, 0)
	end := time.Unix(10, 0)

	mockLabelRequest := func(name string) *logproto.LabelRequest {
		return &logproto.LabelRequest{
			Name:   name,
			Values: name != "",
			Start:  &start,
			End:    &end,
		}
	}

	for _, tc := range []struct {
		desc           string
		name           string
		orgID          string
		expectedLabels []string
	}{
		{
			desc:           "test label request for multiple tenants",
			name:           "test",
			orgID:          "1|2",
			expectedLabels: []string{"test"},
		},
		{
			desc:           "test label request for a single tenant",
			name:           "test",
			orgID:          "1",
			expectedLabels: []string{"test"},
		},
		{
			desc:           "defaultTenantLabel label request for multiple tenants",
			name:           defaultTenantLabel,
			orgID:          "1|2",
			expectedLabels: []string{"1", "2"},
		},
		{
			desc:           "defaultTenantLabel label request for a single tenant",
			name:           defaultTenantLabel,
			orgID:          "1",
			expectedLabels: []string{"1"},
		},
		{
			desc:           "label names for multiple tenants",
			name:           "",
			orgID:          "1|2",
			expectedLabels: []string{defaultTenantLabel, "test"},
		},
		{
			desc:           "label names for a single tenant",
			name:           "",
			orgID:          "1",
			expectedLabels: []string{"test"},
		},
	} {
		t.Run(tc.desc, func(t *testing.T) {
			querier := newQuerierMock()
			querier.On("Label", mock.Anything, mock.Anything).Return(mockLabelResponse([]string{"test"}), nil)
			multiTenantQuerier := NewMultiTenantQuerier(querier, log.NewNopLogger())
			ctx := user.InjectOrgID(context.Background(), tc.orgID)

			resp, err := multiTenantQuerier.Label(ctx, mockLabelRequest(tc.name))
			require.NoError(t, err)
			require.Equal(t, tc.expectedLabels, resp.GetValues())
		})
	}
}

func TestMultiTenantQuerierSeries(t *testing.T) {
	for _, tc := range []struct {
		desc           string
		orgID          string
		expectedSeries []logproto.SeriesIdentifier
	}{
		{
			desc:  "two tenantIDs",
			orgID: "1|2",
			expectedSeries: []logproto.SeriesIdentifier{
				{Labels: logproto.MustNewSeriesEntries("a", "1", "b", "2", "__tenant_id__", "1")},
				{Labels: logproto.MustNewSeriesEntries("a", "1", "b", "3", "__tenant_id__", "1")},
				{Labels: logproto.MustNewSeriesEntries("a", "1", "b", "4", "__tenant_id__", "1")},
				{Labels: logproto.MustNewSeriesEntries("a", "1", "b", "5", "__tenant_id__", "1")},
				{Labels: logproto.MustNewSeriesEntries("a", "1", "b", "2", "__tenant_id__", "2")},
				{Labels: logproto.MustNewSeriesEntries("a", "1", "b", "3", "__tenant_id__", "2")},
				{Labels: logproto.MustNewSeriesEntries("a", "1", "b", "4", "__tenant_id__", "2")},
				{Labels: logproto.MustNewSeriesEntries("a", "1", "b", "5", "__tenant_id__", "2")},
			},
		},
		{
			desc:  "three tenantIDs",
			orgID: "1|2|3",
			expectedSeries: []logproto.SeriesIdentifier{
				{Labels: logproto.MustNewSeriesEntries("a", "1", "b", "2", "__tenant_id__", "1")},
				{Labels: logproto.MustNewSeriesEntries("a", "1", "b", "3", "__tenant_id__", "1")},
				{Labels: logproto.MustNewSeriesEntries("a", "1", "b", "4", "__tenant_id__", "1")},
				{Labels: logproto.MustNewSeriesEntries("a", "1", "b", "5", "__tenant_id__", "1")},
				{Labels: logproto.MustNewSeriesEntries("a", "1", "b", "2", "__tenant_id__", "2")},
				{Labels: logproto.MustNewSeriesEntries("a", "1", "b", "3", "__tenant_id__", "2")},
				{Labels: logproto.MustNewSeriesEntries("a", "1", "b", "4", "__tenant_id__", "2")},
				{Labels: logproto.MustNewSeriesEntries("a", "1", "b", "5", "__tenant_id__", "2")},
				{Labels: logproto.MustNewSeriesEntries("a", "1", "b", "2", "__tenant_id__", "3")},
				{Labels: logproto.MustNewSeriesEntries("a", "1", "b", "3", "__tenant_id__", "3")},
				{Labels: logproto.MustNewSeriesEntries("a", "1", "b", "4", "__tenant_id__", "3")},
				{Labels: logproto.MustNewSeriesEntries("a", "1", "b", "5", "__tenant_id__", "3")},
			},
		},
		{
			desc:  "single tenantID; behaves like a normal `Series` call",
			orgID: "2",
			expectedSeries: []logproto.SeriesIdentifier{
				{Labels: logproto.MustNewSeriesEntries("a", "1", "b", "2")},
				{Labels: logproto.MustNewSeriesEntries("a", "1", "b", "3")},
				{Labels: logproto.MustNewSeriesEntries("a", "1", "b", "4")},
				{Labels: logproto.MustNewSeriesEntries("a", "1", "b", "5")},
			},
		},
	} {
		t.Run(tc.desc, func(t *testing.T) {
			querier := newQuerierMock()
			querier.On("Series", mock.Anything, mock.Anything).Return(func() *logproto.SeriesResponse { return mockSeriesResponse() }, nil)
			multiTenantQuerier := NewMultiTenantQuerier(querier, log.NewNopLogger())
			ctx := user.InjectOrgID(context.Background(), tc.orgID)

			resp, err := multiTenantQuerier.Series(ctx, mockSeriesRequest())
			require.NoError(t, err)
			require.Equal(t, tc.expectedSeries, resp.GetSeries())
		})
	}
}

func TestVolume(t *testing.T) {
	for _, tc := range []struct {
		desc            string
		orgID           string
		expectedVolumes []logproto.Volume
	}{
		{
			desc:  "multiple tenants are aggregated",
			orgID: "1|2",
			expectedVolumes: []logproto.Volume{
				{Name: `{foo="bar"}`, Volume: 76},
			},
		},

		{
			desc:  "single tenant",
			orgID: "2",
			expectedVolumes: []logproto.Volume{
				{Name: `{foo="bar"}`, Volume: 38},
			},
		},
	} {
		t.Run(tc.desc, func(t *testing.T) {
			querier := newQuerierMock()
			querier.On("Volume", mock.Anything, mock.Anything).Return(mockLabelValueResponse(), nil)
			multiTenantQuerier := NewMultiTenantQuerier(querier, log.NewNopLogger())
			ctx := user.InjectOrgID(context.Background(), tc.orgID)

			resp, err := multiTenantQuerier.Volume(ctx, mockLabelValueRequest())
			require.NoError(t, err)
			require.Equal(t, tc.expectedVolumes, resp.GetVolumes())
		})
	}
}

func mockSeriesRequest() *logproto.SeriesRequest {
	return &logproto.SeriesRequest{
		Start: time.Unix(0, 0),
		End:   time.Unix(10, 0),
	}
}

func mockSeriesResponse() *logproto.SeriesResponse {
	return &logproto.SeriesResponse{
		Series: []logproto.SeriesIdentifier{
			{
				Labels: logproto.MustNewSeriesEntries("a", "1", "b", "2"),
			},
			{
				Labels: logproto.MustNewSeriesEntries("a", "1", "b", "3"),
			},
			{
				Labels: logproto.MustNewSeriesEntries("a", "1", "b", "4"),
			},
			{
				Labels: logproto.MustNewSeriesEntries("a", "1", "b", "5"),
			},
		},
	}
}

func mockLabelValueRequest() *logproto.VolumeRequest {
	return &logproto.VolumeRequest{
		From:     0,
		Through:  1000,
		Matchers: `{foo="bar"}`,
		Limit:    10,
	}
}

func mockLabelValueResponse() *logproto.VolumeResponse {
	return &logproto.VolumeResponse{Volumes: []logproto.Volume{
		{Name: `{foo="bar"}`, Volume: 38},
	},
		Limit: 10,
	}
}

func removeWhiteSpace(s string) string {
	return strings.Map(func(r rune) rune {
		if r == ' ' || unicode.IsSpace(r) {
			return -1
		}
		return r
	}, s)
}

func TestSliceToSet(t *testing.T) {
	for _, tc := range []struct {
		desc     string
		slice    []string
		expected map[string]struct{}
	}{
		{
			desc:     "empty slice",
			slice:    []string{},
			expected: map[string]struct{}{},
		},
		{
			desc:     "single element",
			slice:    []string{"a"},
			expected: map[string]struct{}{"a": {}},
		},
		{
			desc:     "multiple elements",
			slice:    []string{"a", "b", "c"},
			expected: map[string]struct{}{"a": {}, "b": {}, "c": {}},
		},
	} {
		t.Run(tc.desc, func(t *testing.T) {
			actual := sliceToSet(tc.slice)
			require.Equal(t, tc.expected, actual)
		})
	}
}

func TestMultiTenantQuerier_DetectedLabels(t *testing.T) {
	for _, tc := range []struct {
		desc      string
		orgID     string
		expected  []*logproto.DetectedLabel
		mockSetup func(*querierMock)
	}{
		{
			desc:  "single tenant",
			orgID: "1",
			expected: []*logproto.DetectedLabel{
				{Label: "app", Cardinality: 100},
				{Label: "env", Cardinality: 50},
			},
			mockSetup: func(q *querierMock) {
				// Create sketches for app and env labels
				appSketch := hyperloglog.New()
				for i := 0; i < 100; i++ {
					appSketch.Insert([]byte(fmt.Sprintf("app-value-%d", i)))
				}
				appSketchData, _ := appSketch.MarshalBinary()

				envSketch := hyperloglog.New()
				for i := 0; i < 50; i++ {
					envSketch.Insert([]byte(fmt.Sprintf("env-value-%d", i)))
				}
				envSketchData, _ := envSketch.MarshalBinary()

				q.On("DetectedLabels", mock.Anything, mock.Anything).Return(&logproto.DetectedLabelsResponse{
					DetectedLabels: []*logproto.DetectedLabel{
						{Label: "app", Cardinality: 100, Sketch: appSketchData},
						{Label: "env", Cardinality: 50, Sketch: envSketchData},
					},
				}, nil)
			},
		},
		{
			desc:  "multiple tenants with overlapping labels",
			orgID: "1|2",
			expected: []*logproto.DetectedLabel{
				{Label: "app", Cardinality: 150},    // Combined cardinality after merging sketches
				{Label: "env", Cardinality: 50},     // from tenant 1
				{Label: "service", Cardinality: 75}, // from tenant 2
			},
			mockSetup: func(q *querierMock) {
				// Create sketches for tenant 1
				appSketch1 := hyperloglog.New()
				for i := 0; i < 100; i++ {
					appSketch1.Insert([]byte(fmt.Sprintf("app-value-%d", i)))
				}
				appSketch1Data, _ := appSketch1.MarshalBinary()

				envSketch := hyperloglog.New()
				for i := 0; i < 50; i++ {
					envSketch.Insert([]byte(fmt.Sprintf("env-value-%d", i)))
				}
				envSketchData, _ := envSketch.MarshalBinary()

				// Create sketches for tenant 2
				appSketch2 := hyperloglog.New()
				for i := 50; i < 150; i++ { // 50 new values + 50 overlapping values
					appSketch2.Insert([]byte(fmt.Sprintf("app-value-%d", i)))
				}
				appSketch2Data, _ := appSketch2.MarshalBinary()

				serviceSketch := hyperloglog.New()
				for i := 0; i < 75; i++ {
					serviceSketch.Insert([]byte(fmt.Sprintf("service-value-%d", i)))
				}
				serviceSketchData, _ := serviceSketch.MarshalBinary()

				q.On("DetectedLabels", mock.MatchedBy(func(ctx context.Context) bool {
					id, err := user.ExtractOrgID(ctx)
					return err == nil && id == "1"
				}), mock.Anything).Return(&logproto.DetectedLabelsResponse{
					DetectedLabels: []*logproto.DetectedLabel{
						{Label: "app", Cardinality: 100, Sketch: appSketch1Data},
						{Label: "env", Cardinality: 50, Sketch: envSketchData},
					},
				}, nil).Once()

				q.On("DetectedLabels", mock.MatchedBy(func(ctx context.Context) bool {
					id, err := user.ExtractOrgID(ctx)
					return err == nil && id == "2"
				}), mock.Anything).Return(&logproto.DetectedLabelsResponse{
					DetectedLabels: []*logproto.DetectedLabel{
						{Label: "app", Cardinality: 100, Sketch: appSketch2Data},
						{Label: "service", Cardinality: 75, Sketch: serviceSketchData},
					},
				}, nil).Once()
			},
		},
		{
			desc:  "multiple tenants with unique labels",
			orgID: "1|2",
			expected: []*logproto.DetectedLabel{
				{Label: "app1", Cardinality: 100},
				{Label: "app2", Cardinality: 200},
				{Label: "env1", Cardinality: 50},
				{Label: "env2", Cardinality: 75},
			},
			mockSetup: func(q *querierMock) {
				// Create sketches for tenant 1
				app1Sketch := hyperloglog.New()
				for i := 0; i < 100; i++ {
					app1Sketch.Insert([]byte(fmt.Sprintf("app1-value-%d", i)))
				}
				app1SketchData, _ := app1Sketch.MarshalBinary()

				env1Sketch := hyperloglog.New()
				for i := 0; i < 50; i++ {
					env1Sketch.Insert([]byte(fmt.Sprintf("env1-value-%d", i)))
				}
				env1SketchData, _ := env1Sketch.MarshalBinary()

				// Create sketches for tenant 2
				app2Sketch := hyperloglog.New()
				for i := 0; i < 200; i++ {
					app2Sketch.Insert([]byte(fmt.Sprintf("app2-value-%d", i)))
				}
				app2SketchData, _ := app2Sketch.MarshalBinary()

				env2Sketch := hyperloglog.New()
				for i := 0; i < 75; i++ {
					env2Sketch.Insert([]byte(fmt.Sprintf("env2-value-%d", i)))
				}
				env2SketchData, _ := env2Sketch.MarshalBinary()

				q.On("DetectedLabels", mock.MatchedBy(func(ctx context.Context) bool {
					id, err := user.ExtractOrgID(ctx)
					return err == nil && id == "1"
				}), mock.Anything).Return(&logproto.DetectedLabelsResponse{
					DetectedLabels: []*logproto.DetectedLabel{
						{Label: "app1", Cardinality: 100, Sketch: app1SketchData},
						{Label: "env1", Cardinality: 50, Sketch: env1SketchData},
					},
				}, nil).Once()

				q.On("DetectedLabels", mock.MatchedBy(func(ctx context.Context) bool {
					id, err := user.ExtractOrgID(ctx)
					return err == nil && id == "2"
				}), mock.Anything).Return(&logproto.DetectedLabelsResponse{
					DetectedLabels: []*logproto.DetectedLabel{
						{Label: "app2", Cardinality: 200, Sketch: app2SketchData},
						{Label: "env2", Cardinality: 75, Sketch: env2SketchData},
					},
				}, nil).Once()
			},
		},
	} {
		t.Run(tc.desc, func(t *testing.T) {
			querier := newQuerierMock()
			tc.mockSetup(querier)

			multiTenantQuerier := NewMultiTenantQuerier(querier, log.NewNopLogger())

			ctx := user.InjectOrgID(context.Background(), tc.orgID)
			req := &logproto.DetectedLabelsRequest{
				Query: `{app="foo"}`,
				Start: time.Now().Add(-1 * time.Hour),
				End:   time.Now(),
			}

			resp, err := multiTenantQuerier.DetectedLabels(ctx, req)
			require.NoError(t, err)
			require.Equal(t, len(tc.expected), len(resp.DetectedLabels))

			// Sort both slices for comparison
			sort.Slice(tc.expected, func(i, j int) bool {
				return tc.expected[i].Label < tc.expected[j].Label
			})
			sort.Slice(resp.DetectedLabels, func(i, j int) bool {
				return resp.DetectedLabels[i].Label < resp.DetectedLabels[j].Label
			})

			for i := range tc.expected {
				require.Equal(t, tc.expected[i].Label, resp.DetectedLabels[i].Label)
				// Allow for some error in cardinality estimation due to HyperLogLog approximation
				require.InDelta(t, tc.expected[i].Cardinality, resp.DetectedLabels[i].Cardinality, float64(tc.expected[i].Cardinality)*0.02)
			}
		})
	}
}
