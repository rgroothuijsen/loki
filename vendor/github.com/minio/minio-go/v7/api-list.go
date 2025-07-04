/*
 * MinIO Go Library for Amazon S3 Compatible Cloud Storage
 * Copyright 2015-2020 MinIO, Inc.
 *
 * Licensed under the Apache License, Version 2.0 (the "License");
 * you may not use this file except in compliance with the License.
 * You may obtain a copy of the License at
 *
 *     http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing, software
 * distributed under the License is distributed on an "AS IS" BASIS,
 * WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 * See the License for the specific language governing permissions and
 * limitations under the License.
 */

package minio

import (
	"context"
	"fmt"
	"iter"
	"net/http"
	"net/url"
	"slices"
	"time"

	"github.com/minio/minio-go/v7/pkg/s3utils"
)

// ListBuckets list all buckets owned by this authenticated user.
//
// This call requires explicit authentication, no anonymous requests are
// allowed for listing buckets.
//
//	api := client.New(....)
//	for message := range api.ListBuckets(context.Background()) {
//	    fmt.Println(message)
//	}
func (c *Client) ListBuckets(ctx context.Context) ([]BucketInfo, error) {
	// Execute GET on service.
	resp, err := c.executeMethod(ctx, http.MethodGet, requestMetadata{contentSHA256Hex: emptySHA256Hex})
	defer closeResponse(resp)
	if err != nil {
		return nil, err
	}
	if resp != nil {
		if resp.StatusCode != http.StatusOK {
			return nil, httpRespToErrorResponse(resp, "", "")
		}
	}
	listAllMyBucketsResult := listAllMyBucketsResult{}
	err = xmlDecoder(resp.Body, &listAllMyBucketsResult)
	if err != nil {
		return nil, err
	}
	return listAllMyBucketsResult.Buckets.Bucket, nil
}

// ListDirectoryBuckets list all buckets owned by this authenticated user.
//
// This call requires explicit authentication, no anonymous requests are
// allowed for listing buckets.
//
// api := client.New(....)
// dirBuckets, err := api.ListDirectoryBuckets(context.Background())
func (c *Client) ListDirectoryBuckets(ctx context.Context) (iter.Seq2[BucketInfo, error], error) {
	fetchBuckets := func(continuationToken string) ([]BucketInfo, string, error) {
		metadata := requestMetadata{contentSHA256Hex: emptySHA256Hex}
		metadata.queryValues = url.Values{}
		metadata.queryValues.Set("max-directory-buckets", "1000")
		if continuationToken != "" {
			metadata.queryValues.Set("continuation-token", continuationToken)
		}

		// Execute GET on service.
		resp, err := c.executeMethod(ctx, http.MethodGet, metadata)
		defer closeResponse(resp)
		if err != nil {
			return nil, "", err
		}
		if resp != nil {
			if resp.StatusCode != http.StatusOK {
				return nil, "", httpRespToErrorResponse(resp, "", "")
			}
		}

		results := listAllMyDirectoryBucketsResult{}
		if err = xmlDecoder(resp.Body, &results); err != nil {
			return nil, "", err
		}

		return results.Buckets.Bucket, results.ContinuationToken, nil
	}

	return func(yield func(BucketInfo, error) bool) {
		var continuationToken string
		for {
			buckets, token, err := fetchBuckets(continuationToken)
			if err != nil {
				yield(BucketInfo{}, err)
				return
			}
			for _, bucket := range buckets {
				if !yield(bucket, nil) {
					return
				}
			}
			if token == "" {
				// nothing to continue
				return
			}
			continuationToken = token
		}
	}, nil
}

// Bucket List Operations.
func (c *Client) listObjectsV2(ctx context.Context, bucketName string, opts ListObjectsOptions) iter.Seq[ObjectInfo] {
	// Default listing is delimited at "/"
	delimiter := "/"
	if opts.Recursive {
		// If recursive we do not delimit.
		delimiter = ""
	}

	// Return object owner information by default
	fetchOwner := true

	return func(yield func(ObjectInfo) bool) {
		if contextCanceled(ctx) {
			return
		}

		// Validate bucket name.
		if err := s3utils.CheckValidBucketName(bucketName); err != nil {
			yield(ObjectInfo{Err: err})
			return
		}

		// Validate incoming object prefix.
		if err := s3utils.CheckValidObjectNamePrefix(opts.Prefix); err != nil {
			yield(ObjectInfo{Err: err})
			return
		}

		// Save continuationToken for next request.
		var continuationToken string
		for {
			if contextCanceled(ctx) {
				return
			}

			// Get list of objects a maximum of 1000 per request.
			result, err := c.listObjectsV2Query(ctx, bucketName, opts.Prefix, continuationToken,
				fetchOwner, opts.WithMetadata, delimiter, opts.StartAfter, opts.MaxKeys, opts.headers)
			if err != nil {
				yield(ObjectInfo{Err: err})
				return
			}

			// If contents are available loop through and send over channel.
			for _, object := range result.Contents {
				object.ETag = trimEtag(object.ETag)
				if !yield(object) {
					return
				}
			}

			// Send all common prefixes if any.
			// NOTE: prefixes are only present if the request is delimited.
			for _, obj := range result.CommonPrefixes {
				if !yield(ObjectInfo{Key: obj.Prefix}) {
					return
				}
			}

			// If continuation token present, save it for next request.
			if result.NextContinuationToken != "" {
				continuationToken = result.NextContinuationToken
			}

			// Listing ends result is not truncated, return right here.
			if !result.IsTruncated {
				return
			}

			// Add this to catch broken S3 API implementations.
			if continuationToken == "" {
				if !yield(ObjectInfo{
					Err: fmt.Errorf("listObjectsV2 is truncated without continuationToken, %s S3 server is buggy", c.endpointURL),
				}) {
					return
				}
			}
		}
	}
}

// listObjectsV2Query - (List Objects V2) - List some or all (up to 1000) of the objects in a bucket.
//
// You can use the request parameters as selection criteria to return a subset of the objects in a bucket.
// request parameters :-
// ---------
// ?prefix - Limits the response to keys that begin with the specified prefix.
// ?continuation-token - Used to continue iterating over a set of objects
// ?metadata - Specifies if we want metadata for the objects as part of list operation.
// ?delimiter - A delimiter is a character you use to group keys.
// ?start-after - Sets a marker to start listing lexically at this key onwards.
// ?max-keys - Sets the maximum number of keys returned in the response body.
func (c *Client) listObjectsV2Query(ctx context.Context, bucketName, objectPrefix, continuationToken string, fetchOwner, metadata bool, delimiter, startAfter string, maxkeys int, headers http.Header) (ListBucketV2Result, error) {
	// Validate bucket name.
	if err := s3utils.CheckValidBucketName(bucketName); err != nil {
		return ListBucketV2Result{}, err
	}
	// Validate object prefix.
	if err := s3utils.CheckValidObjectNamePrefix(objectPrefix); err != nil {
		return ListBucketV2Result{}, err
	}
	// Get resources properly escaped and lined up before
	// using them in http request.
	urlValues := make(url.Values)

	// Always set list-type in ListObjects V2
	urlValues.Set("list-type", "2")

	if metadata {
		urlValues.Set("metadata", "true")
	}

	// Set this conditionally if asked
	if startAfter != "" {
		urlValues.Set("start-after", startAfter)
	}

	// Always set encoding-type in ListObjects V2
	urlValues.Set("encoding-type", "url")

	// Set object prefix, prefix value to be set to empty is okay.
	urlValues.Set("prefix", objectPrefix)

	// Set delimiter, delimiter value to be set to empty is okay.
	urlValues.Set("delimiter", delimiter)

	// Set continuation token
	if continuationToken != "" {
		urlValues.Set("continuation-token", continuationToken)
	}

	// Fetch owner when listing
	if fetchOwner {
		urlValues.Set("fetch-owner", "true")
	}

	// Set max keys.
	if maxkeys > 0 {
		urlValues.Set("max-keys", fmt.Sprintf("%d", maxkeys))
	}

	// Execute GET on bucket to list objects.
	resp, err := c.executeMethod(ctx, http.MethodGet, requestMetadata{
		bucketName:       bucketName,
		queryValues:      urlValues,
		contentSHA256Hex: emptySHA256Hex,
		customHeader:     headers,
	})
	defer closeResponse(resp)
	if err != nil {
		return ListBucketV2Result{}, err
	}
	if resp != nil {
		if resp.StatusCode != http.StatusOK {
			return ListBucketV2Result{}, httpRespToErrorResponse(resp, bucketName, "")
		}
	}

	// Decode listBuckets XML.
	listBucketResult := ListBucketV2Result{}
	if err = xmlDecoder(resp.Body, &listBucketResult); err != nil {
		return listBucketResult, err
	}

	// This is an additional verification check to make
	// sure proper responses are received.
	if listBucketResult.IsTruncated && listBucketResult.NextContinuationToken == "" {
		return listBucketResult, ErrorResponse{
			Code:    NotImplemented,
			Message: "Truncated response should have continuation token set",
		}
	}

	for i, obj := range listBucketResult.Contents {
		listBucketResult.Contents[i].Key, err = decodeS3Name(obj.Key, listBucketResult.EncodingType)
		if err != nil {
			return listBucketResult, err
		}
		listBucketResult.Contents[i].LastModified = listBucketResult.Contents[i].LastModified.Truncate(time.Millisecond)
	}

	for i, obj := range listBucketResult.CommonPrefixes {
		listBucketResult.CommonPrefixes[i].Prefix, err = decodeS3Name(obj.Prefix, listBucketResult.EncodingType)
		if err != nil {
			return listBucketResult, err
		}
	}

	// Success.
	return listBucketResult, nil
}

func (c *Client) listObjects(ctx context.Context, bucketName string, opts ListObjectsOptions) iter.Seq[ObjectInfo] {
	// Default listing is delimited at "/"
	delimiter := "/"
	if opts.Recursive {
		// If recursive we do not delimit.
		delimiter = ""
	}

	return func(yield func(ObjectInfo) bool) {
		if contextCanceled(ctx) {
			return
		}

		// Validate bucket name.
		if err := s3utils.CheckValidBucketName(bucketName); err != nil {
			yield(ObjectInfo{Err: err})
			return
		}

		// Validate incoming object prefix.
		if err := s3utils.CheckValidObjectNamePrefix(opts.Prefix); err != nil {
			yield(ObjectInfo{Err: err})
			return
		}

		marker := opts.StartAfter
		for {
			if contextCanceled(ctx) {
				return
			}

			// Get list of objects a maximum of 1000 per request.
			result, err := c.listObjectsQuery(ctx, bucketName, opts.Prefix, marker, delimiter, opts.MaxKeys, opts.headers)
			if err != nil {
				yield(ObjectInfo{Err: err})
				return
			}

			// If contents are available loop through and send over channel.
			for _, object := range result.Contents {
				// Save the marker.
				marker = object.Key
				object.ETag = trimEtag(object.ETag)
				if !yield(object) {
					return
				}
			}

			// Send all common prefixes if any.
			// NOTE: prefixes are only present if the request is delimited.
			for _, obj := range result.CommonPrefixes {
				if !yield(ObjectInfo{Key: obj.Prefix}) {
					return
				}
			}

			// If next marker present, save it for next request.
			if result.NextMarker != "" {
				marker = result.NextMarker
			}

			// Listing ends result is not truncated, return right here.
			if !result.IsTruncated {
				return
			}
		}
	}
}

func (c *Client) listObjectVersions(ctx context.Context, bucketName string, opts ListObjectsOptions) iter.Seq[ObjectInfo] {
	// Default listing is delimited at "/"
	delimiter := "/"
	if opts.Recursive {
		// If recursive we do not delimit.
		delimiter = ""
	}

	return func(yield func(ObjectInfo) bool) {
		if contextCanceled(ctx) {
			return
		}

		// Validate bucket name.
		if err := s3utils.CheckValidBucketName(bucketName); err != nil {
			yield(ObjectInfo{Err: err})
			return
		}

		// Validate incoming object prefix.
		if err := s3utils.CheckValidObjectNamePrefix(opts.Prefix); err != nil {
			yield(ObjectInfo{Err: err})
			return
		}

		var (
			keyMarker       = ""
			versionIDMarker = ""
			preName         = ""
			preKey          = ""
			perVersions     []Version
			numVersions     int
		)

		send := func(vers []Version) bool {
			if opts.WithVersions && opts.ReverseVersions {
				slices.Reverse(vers)
				numVersions = len(vers)
			}
			for _, version := range vers {
				info := ObjectInfo{
					ETag:              trimEtag(version.ETag),
					Key:               version.Key,
					LastModified:      version.LastModified.Truncate(time.Millisecond),
					Size:              version.Size,
					Owner:             version.Owner,
					StorageClass:      version.StorageClass,
					IsLatest:          version.IsLatest,
					VersionID:         version.VersionID,
					IsDeleteMarker:    version.isDeleteMarker,
					UserTags:          version.UserTags,
					UserMetadata:      version.UserMetadata,
					Internal:          version.Internal,
					NumVersions:       numVersions,
					ChecksumMode:      version.ChecksumType,
					ChecksumCRC32:     version.ChecksumCRC32,
					ChecksumCRC32C:    version.ChecksumCRC32C,
					ChecksumSHA1:      version.ChecksumSHA1,
					ChecksumSHA256:    version.ChecksumSHA256,
					ChecksumCRC64NVME: version.ChecksumCRC64NVME,
				}
				if !yield(info) {
					return false
				}
			}
			return true
		}
		for {
			if contextCanceled(ctx) {
				return
			}

			// Get list of objects a maximum of 1000 per request.
			result, err := c.listObjectVersionsQuery(ctx, bucketName, opts, keyMarker, versionIDMarker, delimiter)
			if err != nil {
				yield(ObjectInfo{Err: err})
				return
			}

			if opts.WithVersions && opts.ReverseVersions {
				for _, version := range result.Versions {
					if preName == "" {
						preName = result.Name
						preKey = version.Key
					}
					if result.Name == preName && preKey == version.Key {
						// If the current name is same as previous name,
						// we need to append the version to the previous version.
						perVersions = append(perVersions, version)
						continue
					}
					// Send the file versions.
					if !send(perVersions) {
						return
					}
					perVersions = perVersions[:0]
					perVersions = append(perVersions, version)
					preName = result.Name
					preKey = version.Key
				}
			} else {
				if !send(result.Versions) {
					return
				}
			}

			// Send all common prefixes if any.
			// NOTE: prefixes are only present if the request is delimited.
			for _, obj := range result.CommonPrefixes {
				if !yield(ObjectInfo{Key: obj.Prefix}) {
					return
				}
			}

			// If next key marker is present, save it for next request.
			if result.NextKeyMarker != "" {
				keyMarker = result.NextKeyMarker
			}

			// If next version id marker is present, save it for next request.
			if result.NextVersionIDMarker != "" {
				versionIDMarker = result.NextVersionIDMarker
			}

			// Listing ends result is not truncated, return right here.
			if !result.IsTruncated {
				// sent the lasted file with versions
				if opts.ReverseVersions && len(perVersions) > 0 {
					if !send(perVersions) {
						return
					}
				}
				return
			}
		}
	}
}

// listObjectVersions - (List Object Versions) - List some or all (up to 1000) of the existing objects
// and their versions in a bucket.
//
// You can use the request parameters as selection criteria to return a subset of the objects in a bucket.
// request parameters :-
// ---------
// ?key-marker - Specifies the key to start with when listing objects in a bucket.
// ?version-id-marker - Specifies the version id marker to start with when listing objects with versions in a bucket.
// ?delimiter - A delimiter is a character you use to group keys.
// ?prefix - Limits the response to keys that begin with the specified prefix.
// ?max-keys - Sets the maximum number of keys returned in the response body.
func (c *Client) listObjectVersionsQuery(ctx context.Context, bucketName string, opts ListObjectsOptions, keyMarker, versionIDMarker, delimiter string) (ListVersionsResult, error) {
	// Validate bucket name.
	if err := s3utils.CheckValidBucketName(bucketName); err != nil {
		return ListVersionsResult{}, err
	}
	// Validate object prefix.
	if err := s3utils.CheckValidObjectNamePrefix(opts.Prefix); err != nil {
		return ListVersionsResult{}, err
	}
	// Get resources properly escaped and lined up before
	// using them in http request.
	urlValues := make(url.Values)

	// Set versions to trigger versioning API
	urlValues.Set("versions", "")

	// Set object prefix, prefix value to be set to empty is okay.
	urlValues.Set("prefix", opts.Prefix)

	// Set delimiter, delimiter value to be set to empty is okay.
	urlValues.Set("delimiter", delimiter)

	// Set object marker.
	if keyMarker != "" {
		urlValues.Set("key-marker", keyMarker)
	}

	// Set max keys.
	if opts.MaxKeys > 0 {
		urlValues.Set("max-keys", fmt.Sprintf("%d", opts.MaxKeys))
	}

	// Set version ID marker
	if versionIDMarker != "" {
		urlValues.Set("version-id-marker", versionIDMarker)
	}

	if opts.WithMetadata {
		urlValues.Set("metadata", "true")
	}

	// Always set encoding-type
	urlValues.Set("encoding-type", "url")

	// Execute GET on bucket to list objects.
	resp, err := c.executeMethod(ctx, http.MethodGet, requestMetadata{
		bucketName:       bucketName,
		queryValues:      urlValues,
		contentSHA256Hex: emptySHA256Hex,
		customHeader:     opts.headers,
	})
	defer closeResponse(resp)
	if err != nil {
		return ListVersionsResult{}, err
	}
	if resp != nil {
		if resp.StatusCode != http.StatusOK {
			return ListVersionsResult{}, httpRespToErrorResponse(resp, bucketName, "")
		}
	}

	// Decode ListVersionsResult XML.
	listObjectVersionsOutput := ListVersionsResult{}
	err = xmlDecoder(resp.Body, &listObjectVersionsOutput)
	if err != nil {
		return ListVersionsResult{}, err
	}

	for i, obj := range listObjectVersionsOutput.Versions {
		listObjectVersionsOutput.Versions[i].Key, err = decodeS3Name(obj.Key, listObjectVersionsOutput.EncodingType)
		if err != nil {
			return listObjectVersionsOutput, err
		}
	}

	for i, obj := range listObjectVersionsOutput.CommonPrefixes {
		listObjectVersionsOutput.CommonPrefixes[i].Prefix, err = decodeS3Name(obj.Prefix, listObjectVersionsOutput.EncodingType)
		if err != nil {
			return listObjectVersionsOutput, err
		}
	}

	if listObjectVersionsOutput.NextKeyMarker != "" {
		listObjectVersionsOutput.NextKeyMarker, err = decodeS3Name(listObjectVersionsOutput.NextKeyMarker, listObjectVersionsOutput.EncodingType)
		if err != nil {
			return listObjectVersionsOutput, err
		}
	}

	return listObjectVersionsOutput, nil
}

// listObjects - (List Objects) - List some or all (up to 1000) of the objects in a bucket.
//
// You can use the request parameters as selection criteria to return a subset of the objects in a bucket.
// request parameters :-
// ---------
// ?marker - Specifies the key to start with when listing objects in a bucket.
// ?delimiter - A delimiter is a character you use to group keys.
// ?prefix - Limits the response to keys that begin with the specified prefix.
// ?max-keys - Sets the maximum number of keys returned in the response body.
func (c *Client) listObjectsQuery(ctx context.Context, bucketName, objectPrefix, objectMarker, delimiter string, maxkeys int, headers http.Header) (ListBucketResult, error) {
	// Validate bucket name.
	if err := s3utils.CheckValidBucketName(bucketName); err != nil {
		return ListBucketResult{}, err
	}
	// Validate object prefix.
	if err := s3utils.CheckValidObjectNamePrefix(objectPrefix); err != nil {
		return ListBucketResult{}, err
	}
	// Get resources properly escaped and lined up before
	// using them in http request.
	urlValues := make(url.Values)

	// Set object prefix, prefix value to be set to empty is okay.
	urlValues.Set("prefix", objectPrefix)

	// Set delimiter, delimiter value to be set to empty is okay.
	urlValues.Set("delimiter", delimiter)

	// Set object marker.
	if objectMarker != "" {
		urlValues.Set("marker", objectMarker)
	}

	// Set max keys.
	if maxkeys > 0 {
		urlValues.Set("max-keys", fmt.Sprintf("%d", maxkeys))
	}

	// Always set encoding-type
	urlValues.Set("encoding-type", "url")

	// Execute GET on bucket to list objects.
	resp, err := c.executeMethod(ctx, http.MethodGet, requestMetadata{
		bucketName:       bucketName,
		queryValues:      urlValues,
		contentSHA256Hex: emptySHA256Hex,
		customHeader:     headers,
	})
	defer closeResponse(resp)
	if err != nil {
		return ListBucketResult{}, err
	}
	if resp != nil {
		if resp.StatusCode != http.StatusOK {
			return ListBucketResult{}, httpRespToErrorResponse(resp, bucketName, "")
		}
	}
	// Decode listBuckets XML.
	listBucketResult := ListBucketResult{}
	err = xmlDecoder(resp.Body, &listBucketResult)
	if err != nil {
		return listBucketResult, err
	}

	for i, obj := range listBucketResult.Contents {
		listBucketResult.Contents[i].Key, err = decodeS3Name(obj.Key, listBucketResult.EncodingType)
		if err != nil {
			return listBucketResult, err
		}
		listBucketResult.Contents[i].LastModified = listBucketResult.Contents[i].LastModified.Truncate(time.Millisecond)
	}

	for i, obj := range listBucketResult.CommonPrefixes {
		listBucketResult.CommonPrefixes[i].Prefix, err = decodeS3Name(obj.Prefix, listBucketResult.EncodingType)
		if err != nil {
			return listBucketResult, err
		}
	}

	if listBucketResult.NextMarker != "" {
		listBucketResult.NextMarker, err = decodeS3Name(listBucketResult.NextMarker, listBucketResult.EncodingType)
		if err != nil {
			return listBucketResult, err
		}
	}

	return listBucketResult, nil
}

// ListObjectsOptions holds all options of a list object request
type ListObjectsOptions struct {
	// ReverseVersions - reverse the order of the object versions
	ReverseVersions bool
	// Include objects versions in the listing
	WithVersions bool
	// Include objects metadata in the listing
	WithMetadata bool
	// Only list objects with the prefix
	Prefix string
	// Ignore '/' delimiter
	Recursive bool
	// The maximum number of objects requested per
	// batch, advanced use-case not useful for most
	// applications
	MaxKeys int
	// StartAfter start listing lexically at this
	// object onwards, this value can also be set
	// for Marker when `UseV1` is set to true.
	StartAfter string

	// Use the deprecated list objects V1 API
	UseV1 bool

	headers http.Header
}

// Set adds a key value pair to the options. The
// key-value pair will be part of the HTTP GET request
// headers.
func (o *ListObjectsOptions) Set(key, value string) {
	if o.headers == nil {
		o.headers = make(http.Header)
	}
	o.headers.Set(key, value)
}

// ListObjects returns objects list after evaluating the passed options.
//
//	api := client.New(....)
//	for object := range api.ListObjects(ctx, "mytestbucket", minio.ListObjectsOptions{Prefix: "starthere", Recursive:true}) {
//	    fmt.Println(object)
//	}
//
// If caller cancels the context, then the last entry on the 'chan ObjectInfo' will be the context.Error()
// caller must drain the channel entirely and wait until channel is closed before proceeding, without
// waiting on the channel to be closed completely you might leak goroutines.
func (c *Client) ListObjects(ctx context.Context, bucketName string, opts ListObjectsOptions) <-chan ObjectInfo {
	objectStatCh := make(chan ObjectInfo, 1)
	go func() {
		defer close(objectStatCh)
		if contextCanceled(ctx) {
			objectStatCh <- ObjectInfo{Err: ctx.Err()}
			return
		}

		var objIter iter.Seq[ObjectInfo]
		switch {
		case opts.WithVersions:
			objIter = c.listObjectVersions(ctx, bucketName, opts)
		case opts.UseV1:
			objIter = c.listObjects(ctx, bucketName, opts)
		default:
			location, _ := c.bucketLocCache.Get(bucketName)
			if location == "snowball" {
				objIter = c.listObjects(ctx, bucketName, opts)
			} else {
				objIter = c.listObjectsV2(ctx, bucketName, opts)
			}
		}
		for obj := range objIter {
			select {
			case <-ctx.Done():
				objectStatCh <- ObjectInfo{Err: ctx.Err()}
				return
			case objectStatCh <- obj:
			}
		}
	}()
	return objectStatCh
}

// ListObjectsIter returns object list as a iterator sequence.
// caller must cancel the context if they are not interested in
// iterating further, if no more entries the iterator will
// automatically stop.
//
//	api := client.New(....)
//	for object := range api.ListObjectsIter(ctx, "mytestbucket", minio.ListObjectsOptions{Prefix: "starthere", Recursive:true}) {
//	    if object.Err != nil {
//	        // handle the errors.
//	    }
//	    fmt.Println(object)
//	}
//
// Canceling the context the iterator will stop, if you wish to discard the yielding make sure
// to cancel the passed context without that you might leak coroutines
func (c *Client) ListObjectsIter(ctx context.Context, bucketName string, opts ListObjectsOptions) iter.Seq[ObjectInfo] {
	if opts.WithVersions {
		return c.listObjectVersions(ctx, bucketName, opts)
	}

	// Use legacy list objects v1 API
	if opts.UseV1 {
		return c.listObjects(ctx, bucketName, opts)
	}

	// Check whether this is snowball region, if yes ListObjectsV2 doesn't work, fallback to listObjectsV1.
	if location, ok := c.bucketLocCache.Get(bucketName); ok {
		if location == "snowball" {
			return c.listObjects(ctx, bucketName, opts)
		}
	}

	return c.listObjectsV2(ctx, bucketName, opts)
}

// ListIncompleteUploads - List incompletely uploaded multipart objects.
//
// ListIncompleteUploads lists all incompleted objects matching the
// objectPrefix from the specified bucket. If recursion is enabled
// it would list all subdirectories and all its contents.
//
// Your input parameters are just bucketName, objectPrefix, recursive.
// If you enable recursive as 'true' this function will return back all
// the multipart objects in a given bucket name.
//
//	api := client.New(....)
//	// Recurively list all objects in 'mytestbucket'
//	recursive := true
//	for message := range api.ListIncompleteUploads(context.Background(), "mytestbucket", "starthere", recursive) {
//	    fmt.Println(message)
//	}
func (c *Client) ListIncompleteUploads(ctx context.Context, bucketName, objectPrefix string, recursive bool) <-chan ObjectMultipartInfo {
	return c.listIncompleteUploads(ctx, bucketName, objectPrefix, recursive)
}

// contextCanceled returns whether a context is canceled.
func contextCanceled(ctx context.Context) bool {
	select {
	case <-ctx.Done():
		return true
	default:
		return false
	}
}

// listIncompleteUploads lists all incomplete uploads.
func (c *Client) listIncompleteUploads(ctx context.Context, bucketName, objectPrefix string, recursive bool) <-chan ObjectMultipartInfo {
	// Allocate channel for multipart uploads.
	objectMultipartStatCh := make(chan ObjectMultipartInfo, 1)
	// Delimiter is set to "/" by default.
	delimiter := "/"
	if recursive {
		// If recursive do not delimit.
		delimiter = ""
	}
	// Validate bucket name.
	if err := s3utils.CheckValidBucketName(bucketName); err != nil {
		defer close(objectMultipartStatCh)
		objectMultipartStatCh <- ObjectMultipartInfo{
			Err: err,
		}
		return objectMultipartStatCh
	}
	// Validate incoming object prefix.
	if err := s3utils.CheckValidObjectNamePrefix(objectPrefix); err != nil {
		defer close(objectMultipartStatCh)
		objectMultipartStatCh <- ObjectMultipartInfo{
			Err: err,
		}
		return objectMultipartStatCh
	}
	go func(objectMultipartStatCh chan<- ObjectMultipartInfo) {
		defer func() {
			if contextCanceled(ctx) {
				objectMultipartStatCh <- ObjectMultipartInfo{
					Err: ctx.Err(),
				}
			}
			close(objectMultipartStatCh)
		}()

		// object and upload ID marker for future requests.
		var objectMarker string
		var uploadIDMarker string
		for {
			// list all multipart uploads.
			result, err := c.listMultipartUploadsQuery(ctx, bucketName, objectMarker, uploadIDMarker, objectPrefix, delimiter, 0)
			if err != nil {
				objectMultipartStatCh <- ObjectMultipartInfo{
					Err: err,
				}
				return
			}
			objectMarker = result.NextKeyMarker
			uploadIDMarker = result.NextUploadIDMarker

			// Send all multipart uploads.
			for _, obj := range result.Uploads {
				// Calculate total size of the uploaded parts if 'aggregateSize' is enabled.
				select {
				// Send individual uploads here.
				case objectMultipartStatCh <- obj:
				// If the context is canceled
				case <-ctx.Done():
					return
				}
			}
			// Send all common prefixes if any.
			// NOTE: prefixes are only present if the request is delimited.
			for _, obj := range result.CommonPrefixes {
				select {
				// Send delimited prefixes here.
				case objectMultipartStatCh <- ObjectMultipartInfo{Key: obj.Prefix, Size: 0}:
				// If context is canceled.
				case <-ctx.Done():
					return
				}
			}
			// Listing ends if result not truncated, return right here.
			if !result.IsTruncated {
				return
			}
		}
	}(objectMultipartStatCh)
	// return.
	return objectMultipartStatCh
}

// listMultipartUploadsQuery - (List Multipart Uploads).
//   - Lists some or all (up to 1000) in-progress multipart uploads in a bucket.
//
// You can use the request parameters as selection criteria to return a subset of the uploads in a bucket.
// request parameters. :-
// ---------
// ?key-marker - Specifies the multipart upload after which listing should begin.
// ?upload-id-marker - Together with key-marker specifies the multipart upload after which listing should begin.
// ?delimiter - A delimiter is a character you use to group keys.
// ?prefix - Limits the response to keys that begin with the specified prefix.
// ?max-uploads - Sets the maximum number of multipart uploads returned in the response body.
func (c *Client) listMultipartUploadsQuery(ctx context.Context, bucketName, keyMarker, uploadIDMarker, prefix, delimiter string, maxUploads int) (ListMultipartUploadsResult, error) {
	// Get resources properly escaped and lined up before using them in http request.
	urlValues := make(url.Values)
	// Set uploads.
	urlValues.Set("uploads", "")
	// Set object key marker.
	if keyMarker != "" {
		urlValues.Set("key-marker", keyMarker)
	}
	// Set upload id marker.
	if uploadIDMarker != "" {
		urlValues.Set("upload-id-marker", uploadIDMarker)
	}

	// Set object prefix, prefix value to be set to empty is okay.
	urlValues.Set("prefix", prefix)

	// Set delimiter, delimiter value to be set to empty is okay.
	urlValues.Set("delimiter", delimiter)

	// Always set encoding-type
	urlValues.Set("encoding-type", "url")

	// maxUploads should be 1000 or less.
	if maxUploads > 0 {
		// Set max-uploads.
		urlValues.Set("max-uploads", fmt.Sprintf("%d", maxUploads))
	}

	// Execute GET on bucketName to list multipart uploads.
	resp, err := c.executeMethod(ctx, http.MethodGet, requestMetadata{
		bucketName:       bucketName,
		queryValues:      urlValues,
		contentSHA256Hex: emptySHA256Hex,
	})
	defer closeResponse(resp)
	if err != nil {
		return ListMultipartUploadsResult{}, err
	}
	if resp != nil {
		if resp.StatusCode != http.StatusOK {
			return ListMultipartUploadsResult{}, httpRespToErrorResponse(resp, bucketName, "")
		}
	}
	// Decode response body.
	listMultipartUploadsResult := ListMultipartUploadsResult{}
	err = xmlDecoder(resp.Body, &listMultipartUploadsResult)
	if err != nil {
		return listMultipartUploadsResult, err
	}

	listMultipartUploadsResult.NextKeyMarker, err = decodeS3Name(listMultipartUploadsResult.NextKeyMarker, listMultipartUploadsResult.EncodingType)
	if err != nil {
		return listMultipartUploadsResult, err
	}

	listMultipartUploadsResult.NextUploadIDMarker, err = decodeS3Name(listMultipartUploadsResult.NextUploadIDMarker, listMultipartUploadsResult.EncodingType)
	if err != nil {
		return listMultipartUploadsResult, err
	}

	for i, obj := range listMultipartUploadsResult.Uploads {
		listMultipartUploadsResult.Uploads[i].Key, err = decodeS3Name(obj.Key, listMultipartUploadsResult.EncodingType)
		if err != nil {
			return listMultipartUploadsResult, err
		}
	}

	for i, obj := range listMultipartUploadsResult.CommonPrefixes {
		listMultipartUploadsResult.CommonPrefixes[i].Prefix, err = decodeS3Name(obj.Prefix, listMultipartUploadsResult.EncodingType)
		if err != nil {
			return listMultipartUploadsResult, err
		}
	}

	return listMultipartUploadsResult, nil
}

// listObjectParts list all object parts recursively.
//
//lint:ignore U1000 Keep this around
func (c *Client) listObjectParts(ctx context.Context, bucketName, objectName, uploadID string) (partsInfo map[int]ObjectPart, err error) {
	// Part number marker for the next batch of request.
	var nextPartNumberMarker int
	partsInfo = make(map[int]ObjectPart)
	for {
		// Get list of uploaded parts a maximum of 1000 per request.
		listObjPartsResult, err := c.listObjectPartsQuery(ctx, bucketName, objectName, uploadID, nextPartNumberMarker, 1000)
		if err != nil {
			return nil, err
		}
		// Append to parts info.
		for _, part := range listObjPartsResult.ObjectParts {
			// Trim off the odd double quotes from ETag in the beginning and end.
			part.ETag = trimEtag(part.ETag)
			partsInfo[part.PartNumber] = part
		}
		// Keep part number marker, for the next iteration.
		nextPartNumberMarker = listObjPartsResult.NextPartNumberMarker
		// Listing ends result is not truncated, return right here.
		if !listObjPartsResult.IsTruncated {
			break
		}
	}

	// Return all the parts.
	return partsInfo, nil
}

// findUploadIDs lists all incomplete uploads and find the uploadIDs of the matching object name.
func (c *Client) findUploadIDs(ctx context.Context, bucketName, objectName string) ([]string, error) {
	var uploadIDs []string
	// Make list incomplete uploads recursive.
	isRecursive := true
	// List all incomplete uploads.
	for mpUpload := range c.listIncompleteUploads(ctx, bucketName, objectName, isRecursive) {
		if mpUpload.Err != nil {
			return nil, mpUpload.Err
		}
		if objectName == mpUpload.Key {
			uploadIDs = append(uploadIDs, mpUpload.UploadID)
		}
	}
	// Return the latest upload id.
	return uploadIDs, nil
}

// listObjectPartsQuery (List Parts query)
//   - lists some or all (up to 1000) parts that have been uploaded
//     for a specific multipart upload
//
// You can use the request parameters as selection criteria to return
// a subset of the uploads in a bucket, request parameters :-
// ---------
// ?part-number-marker - Specifies the part after which listing should
// begin.
// ?max-parts - Maximum parts to be listed per request.
func (c *Client) listObjectPartsQuery(ctx context.Context, bucketName, objectName, uploadID string, partNumberMarker, maxParts int) (ListObjectPartsResult, error) {
	// Get resources properly escaped and lined up before using them in http request.
	urlValues := make(url.Values)
	// Set part number marker.
	urlValues.Set("part-number-marker", fmt.Sprintf("%d", partNumberMarker))
	// Set upload id.
	urlValues.Set("uploadId", uploadID)

	// maxParts should be 1000 or less.
	if maxParts > 0 {
		// Set max parts.
		urlValues.Set("max-parts", fmt.Sprintf("%d", maxParts))
	}

	// Execute GET on objectName to get list of parts.
	resp, err := c.executeMethod(ctx, http.MethodGet, requestMetadata{
		bucketName:       bucketName,
		objectName:       objectName,
		queryValues:      urlValues,
		contentSHA256Hex: emptySHA256Hex,
	})
	defer closeResponse(resp)
	if err != nil {
		return ListObjectPartsResult{}, err
	}
	if resp != nil {
		if resp.StatusCode != http.StatusOK {
			return ListObjectPartsResult{}, httpRespToErrorResponse(resp, bucketName, objectName)
		}
	}
	// Decode list object parts XML.
	listObjectPartsResult := ListObjectPartsResult{}
	err = xmlDecoder(resp.Body, &listObjectPartsResult)
	if err != nil {
		return listObjectPartsResult, err
	}
	return listObjectPartsResult, nil
}

// Decode an S3 object name according to the encoding type
func decodeS3Name(name, encodingType string) (string, error) {
	switch encodingType {
	case "url":
		return url.QueryUnescape(name)
	default:
		return name, nil
	}
}
