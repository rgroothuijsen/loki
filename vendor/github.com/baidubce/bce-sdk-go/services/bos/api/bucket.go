/*
 * Copyright 2017 Baidu, Inc.
 *
 * Licensed under the Apache License, Version 2.0 (the "License"); you may not use this file
 * except in compliance with the License. You may obtain a copy of the License at
 *
 * http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing, software distributed under the
 * License is distributed on an "AS IS" BASIS, WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND,
 * either express or implied. See the License for the specific language governing permissions
 * and limitations under the License.
 */

// bucket.go - the bucket APIs definition supported by the BOS service

// Package api defines all APIs supported by the BOS service of BCE.
package api

import (
	"encoding/json"
	"fmt"
	"strconv"

	"github.com/baidubce/bce-sdk-go/bce"
	"github.com/baidubce/bce-sdk-go/http"
)

// ListBuckets - list all buckets of the account
//
// PARAMS:
//   - cli: the client agent which can perform sending request
//
// RETURNS:
//   - *ListBucketsResult: the result bucket list structure
//   - error: nil if ok otherwise the specific error
func ListBuckets(cli bce.Client, ctx *BosContext, options ...Option) (*ListBucketsResult, error) {
	req := &BosRequest{}
	req.SetMethod(http.GET)
	resp := &BosResponse{}
	// handle options to set the header/params of request
	if err := handleOptions(req, options); err != nil {
		return nil, bce.NewBceClientError(fmt.Sprintf("Handle options occur error: %s", err))
	}
	if err := SendRequest(cli, req, resp, ctx); err != nil {
		return nil, err
	}
	if resp.IsFail() {
		return nil, resp.ServiceError()
	}
	result := &ListBucketsResult{}
	if err := resp.ParseJsonBody(result); err != nil {
		return nil, err
	}
	return result, nil
}

// ListObjects - list all objects of the given bucket
//
// PARAMS:
//   - cli: the client agent which can perform sending request
//   - bucket: the bucket name
//   - args: the optional arguments to list objects
//
// RETURNS:
//   - *ListObjectsResult: the result object list structure
//   - error: nil if ok otherwise the specific error
func ListObjects(cli bce.Client, bucket string, args *ListObjectsArgs,
	ctx *BosContext, options ...Option) (*ListObjectsResult, error) {
	req := &BosRequest{}
	req.SetUri(getBucketUri(bucket))
	req.SetMethod(http.GET)
	req.SetBucket(bucket)
	// Optional arguments settings
	if args != nil {
		if len(args.Delimiter) != 0 {
			req.SetParam("delimiter", args.Delimiter)
		}
		if len(args.Marker) != 0 {
			req.SetParam("marker", args.Marker)
		}
		if args.MaxKeys != 0 {
			req.SetParam("maxKeys", strconv.Itoa(args.MaxKeys))
		}
		if len(args.Prefix) != 0 {
			req.SetParam("prefix", args.Prefix)
		}
	}
	if args == nil || args.MaxKeys == 0 {
		req.SetParam("maxKeys", "1000")
	}
	// handle options to set the header/params of request
	if err := handleOptions(req, options); err != nil {
		return nil, bce.NewBceClientError(fmt.Sprintf("Handle options occur error: %s", err))
	}
	// Send the request and get result
	resp := &BosResponse{}
	if err := SendRequest(cli, req, resp, ctx); err != nil {
		return nil, err
	}
	if resp.IsFail() {
		return nil, resp.ServiceError()
	}
	result := &ListObjectsResult{}
	if err := resp.ParseJsonBody(result); err != nil {
		return nil, err
	}
	defer func() { resp.Body().Close() }()
	return result, nil
}

func ListObjectsVersions(cli bce.Client, bucket string, args *ListObjectsArgs,
	ctx *BosContext, options ...Option) (*ListObjectsResult, error) {
	req := &BosRequest{}
	req.SetUri(getBucketUri(bucket))
	req.SetMethod(http.GET)
	req.SetParam("versions", "")
	req.SetBucket(bucket)
	// Optional arguments settings
	if args != nil {
		if len(args.Delimiter) != 0 {
			req.SetParam("delimiter", args.Delimiter)
		}
		if len(args.Marker) != 0 {
			req.SetParam("marker", args.Marker)
		}
		if args.MaxKeys != 0 {
			req.SetParam("maxKeys", strconv.Itoa(args.MaxKeys))
		}
		if len(args.Prefix) != 0 {
			req.SetParam("prefix", args.Prefix)
		}
		if len(args.VersionIdMarker) != 0 {
			req.SetParam("versionIdMarker", args.VersionIdMarker)
		}
	}
	if args == nil || args.MaxKeys == 0 {
		req.SetParam("maxKeys", "1000")
	}
	// handle options to set the header/params of request
	if err := handleOptions(req, options); err != nil {
		return nil, bce.NewBceClientError(fmt.Sprintf("Handle options occur error: %s", err))
	}
	// Send the request and get result
	resp := &BosResponse{}
	if err := SendRequest(cli, req, resp, ctx); err != nil {
		return nil, err
	}
	if resp.IsFail() {
		return nil, resp.ServiceError()
	}
	result := &ListObjectsResult{}
	if err := resp.ParseJsonBody(result); err != nil {
		return nil, err
	}
	defer func() { resp.Body().Close() }()
	return result, nil
}

// HeadBucket - test the given bucket existed and access authority
//
// PARAMS:
//   - cli: the client agent which can perform sending request
//   - bucket: the bucket name
//
// RETURNS:
//   - error: nil if exists and have authority otherwise the specific error
func HeadBucket(cli bce.Client, bucket string, ctx *BosContext, options ...Option) (error, *BosResponse) {
	req := &BosRequest{}
	req.SetUri(getBucketUri(bucket))
	req.SetMethod(http.HEAD)
	req.SetBucket(bucket)
	resp := &BosResponse{}
	// handle options to set the header/params of request
	if err := handleOptions(req, options); err != nil {
		return bce.NewBceClientError(fmt.Sprintf("Handle options occur error: %s", err)), nil
	}
	if err := SendRequest(cli, req, resp, ctx); err != nil {
		return err, resp
	}
	if resp.IsFail() {
		return resp.ServiceError(), resp
	}
	defer func() { resp.Body().Close() }()
	return nil, resp
}

// PutBucket - create a new bucket with the given name
//
// PARAMS:
//   - cli: the client agent which can perform sending request
//   - bucket: the new bucket name
//
// RETURNS:
//   - string: the location of the new bucket if create success
//   - error: nil if create success otherwise the specific error
func PutBucket(cli bce.Client, bucket string, args *PutBucketArgs,
	ctx *BosContext, options ...Option) (string, error) {
	req := &BosRequest{}
	req.SetUri(getBucketUri(bucket))
	req.SetMethod(http.PUT)
	req.SetBucket(bucket)
	if args != nil {
		if len(args.TagList) != 0 {
			req.SetHeader(http.BCE_TAG, args.TagList)
		}
		jsonBytes, jsonErr := json.Marshal(args)
		if jsonErr != nil {
			return "", jsonErr
		}
		body, err := bce.NewBodyFromBytes(jsonBytes)
		if err != nil {
			return "", err
		}
		req.SetBody(body)
	}
	// handle options to set the header/params of request
	if err := handleOptions(req, options); err != nil {
		return "", bce.NewBceClientError(fmt.Sprintf("Handle options occur error: %s", err))
	}
	resp := &BosResponse{}
	if err := SendRequest(cli, req, resp, ctx); err != nil {
		return "", err
	}
	if resp.IsFail() {
		return "", resp.ServiceError()
	}
	defer func() { resp.Body().Close() }()
	return resp.Header(http.LOCATION), nil
}

// DeleteBucket - delete an empty bucket by given name
//
// PARAMS:
//   - cli: the client agent which can perform sending request
//   - bucket: the bucket name to be deleted
//
// RETURNS:
//   - error: nil if delete success otherwise the specific error
func DeleteBucket(cli bce.Client, bucket string, ctx *BosContext, options ...Option) error {
	req := &BosRequest{}
	req.SetUri(getBucketUri(bucket))
	req.SetMethod(http.DELETE)
	req.SetBucket(bucket)
	resp := &BosResponse{}
	// handle options to set the header/params of request
	if err := handleOptions(req, options); err != nil {
		return bce.NewBceClientError(fmt.Sprintf("Handle options occur error: %s", err))
	}
	if err := SendRequest(cli, req, resp, ctx); err != nil {
		return err
	}
	if resp.IsFail() {
		return resp.ServiceError()
	}
	defer func() { resp.Body().Close() }()
	return nil
}

// GetBucketLocation - get the location of the given bucket
//
// PARAMS:
//   - cli: the client agent which can perform sending request
//   - bucket: the bucket name
//
// RETURNS:
//   - string: the location of the bucket
//   - error: nil if delete success otherwise the specific error
func GetBucketLocation(cli bce.Client, bucket string, ctx *BosContext, options ...Option) (string, error) {
	req := &BosRequest{}
	req.SetUri(getBucketUri(bucket))
	req.SetMethod(http.GET)
	req.SetParam("location", "")
	req.SetBucket(bucket)
	resp := &BosResponse{}
	// handle options to set the header/params of request
	if err := handleOptions(req, options); err != nil {
		return "", bce.NewBceClientError(fmt.Sprintf("Handle options occur error: %s", err))
	}
	if err := SendRequest(cli, req, resp, ctx); err != nil {
		return "", err
	}
	if resp.IsFail() {
		return "", resp.ServiceError()
	}
	result := &LocationType{}
	if err := resp.ParseJsonBody(result); err != nil {
		return "", err
	}
	defer func() { resp.Body().Close() }()
	return result.LocationConstraint, nil
}

// PutBucketAcl - set the acl of the given bucket
//
// PARAMS:
//   - cli: the client agent which can perform sending request
//   - bucket: the bucket name
//   - cannedAcl: support private, public-read, public-read-write
//   - aclBody: the acl file body
//
// RETURNS:
//   - error: nil if delete success otherwise the specific error
func PutBucketAcl(cli bce.Client, bucket, cannedAcl string, aclBody *bce.Body,
	ctx *BosContext, options ...Option) error {
	req := &BosRequest{}
	req.SetUri(getBucketUri(bucket))
	req.SetMethod(http.PUT)
	req.SetParam("acl", "")
	req.SetBucket(bucket)
	// The acl setting
	if len(cannedAcl) != 0 && aclBody != nil {
		return bce.NewBceClientError("BOS does not support cannedAcl and acl file at the same time")
	}
	if validCannedAcl(cannedAcl) {
		req.SetHeader(http.BCE_ACL, cannedAcl)
	}
	if aclBody != nil {
		req.SetHeader(http.CONTENT_TYPE, bce.DEFAULT_CONTENT_TYPE)
		req.SetBody(aclBody)
	}
	// handle options to set the header/params of request
	if err := handleOptions(req, options); err != nil {
		return bce.NewBceClientError(fmt.Sprintf("Handle options occur error: %s", err))
	}
	resp := &BosResponse{}
	if err := SendRequest(cli, req, resp, ctx); err != nil {
		return err
	}
	if resp.IsFail() {
		return resp.ServiceError()
	}
	defer func() { resp.Body().Close() }()
	return nil
}

// GetBucketAcl - get the acl of the given bucket
//
// PARAMS:
//   - cli: the client agent which can perform sending request
//   - bucket: the bucket name
//
// RETURNS:
//   - *GetBucketAclResult: the result of the bucket acl
//   - error: nil if success otherwise the specific error
func GetBucketAcl(cli bce.Client, bucket string, ctx *BosContext,
	options ...Option) (*GetBucketAclResult, error) {
	req := &BosRequest{}
	req.SetUri(getBucketUri(bucket))
	req.SetMethod(http.GET)
	req.SetParam("acl", "")
	req.SetBucket(bucket)
	resp := &BosResponse{}
	// handle options to set the header/params of request
	if err := handleOptions(req, options); err != nil {
		return nil, bce.NewBceClientError(fmt.Sprintf("Handle options occur error: %s", err))
	}
	if err := SendRequest(cli, req, resp, ctx); err != nil {
		return nil, err
	}
	if resp.IsFail() {
		return nil, resp.ServiceError()
	}
	result := &GetBucketAclResult{}
	if err := resp.ParseJsonBody(result); err != nil {
		return nil, err
	}
	defer func() { resp.Body().Close() }()
	return result, nil
}

// PutBucketLogging - set the logging prefix of the given bucket
//
// PARAMS:
//   - cli: the client agent which can perform sending request
//   - bucket: the bucket name
//   - logging: the logging prefix json string body
//
// RETURNS:
//   - error: nil if success otherwise the specific error
func PutBucketLogging(cli bce.Client, bucket string, logging *bce.Body,
	ctx *BosContext, options ...Option) error {
	req := &BosRequest{}
	req.SetUri(getBucketUri(bucket))
	req.SetMethod(http.PUT)
	req.SetParam("logging", "")
	req.SetBody(logging)
	req.SetBucket(bucket)
	resp := &BosResponse{}
	// handle options to set the header/params of request
	if err := handleOptions(req, options); err != nil {
		return bce.NewBceClientError(fmt.Sprintf("Handle options occur error: %s", err))
	}
	if err := SendRequest(cli, req, resp, ctx); err != nil {
		return err
	}
	if resp.IsFail() {
		return resp.ServiceError()
	}
	defer func() { resp.Body().Close() }()
	return nil
}

// GetBucketLogging - get the logging config of the given bucket
//
// PARAMS:
//   - cli: the client agent which can perform sending request
//   - bucket: the bucket name
//
// RETURNS:
//   - *GetBucketLoggingResult: the logging setting of the bucket
//   - error: nil if success otherwise the specific error
func GetBucketLogging(cli bce.Client, bucket string, ctx *BosContext,
	options ...Option) (*GetBucketLoggingResult, error) {
	req := &BosRequest{}
	req.SetUri(getBucketUri(bucket))
	req.SetMethod(http.GET)
	req.SetParam("logging", "")
	req.SetBucket(bucket)
	// handle options to set the header/params of request
	if err := handleOptions(req, options); err != nil {
		return nil, bce.NewBceClientError(fmt.Sprintf("Handle options occur error: %s", err))
	}
	resp := &BosResponse{}
	if err := SendRequest(cli, req, resp, ctx); err != nil {
		return nil, err
	}
	if resp.IsFail() {
		return nil, resp.ServiceError()
	}
	result := &GetBucketLoggingResult{}
	if err := resp.ParseJsonBody(result); err != nil {
		return nil, err
	}
	return result, nil
}

// DeleteBucketLogging - delete the logging setting of the given bucket
//
// PARAMS:
//   - cli: the client agent which can perform sending request
//   - bucket: the bucket name
//
// RETURNS:
//   - error: nil if success otherwise the specific error
func DeleteBucketLogging(cli bce.Client, bucket string, ctx *BosContext, options ...Option) error {
	req := &BosRequest{}
	req.SetUri(getBucketUri(bucket))
	req.SetMethod(http.DELETE)
	req.SetParam("logging", "")
	req.SetBucket(bucket)
	// handle options to set the header/params of request
	if err := handleOptions(req, options); err != nil {
		return bce.NewBceClientError(fmt.Sprintf("Handle options occur error: %s", err))
	}
	resp := &BosResponse{}
	if err := SendRequest(cli, req, resp, ctx); err != nil {
		return err
	}
	if resp.IsFail() {
		return resp.ServiceError()
	}
	defer func() { resp.Body().Close() }()
	return nil
}

// PutBucketLifecycle - set the lifecycle rule of the given bucket
//
// PARAMS:
//   - cli: the client agent which can perform sending request
//   - bucket: the bucket name
//   - lifecycle: the lifecycle rule json string body
//
// RETURNS:
//   - error: nil if success otherwise the specific error
func PutBucketLifecycle(cli bce.Client, bucket string, lifecycle *bce.Body,
	ctx *BosContext, options ...Option) error {
	req := &BosRequest{}
	req.SetUri(getBucketUri(bucket))
	req.SetMethod(http.PUT)
	req.SetParam("lifecycle", "")
	req.SetBody(lifecycle)
	req.SetBucket(bucket)
	// handle options to set the header/params of request
	if err := handleOptions(req, options); err != nil {
		return bce.NewBceClientError(fmt.Sprintf("Handle options occur error: %s", err))
	}
	resp := &BosResponse{}
	if err := SendRequest(cli, req, resp, ctx); err != nil {
		return err
	}
	if resp.IsFail() {
		return resp.ServiceError()
	}
	defer func() { resp.Body().Close() }()
	return nil
}

// GetBucketLifecycle - get the lifecycle rule of the given bucket
//
// PARAMS:
//   - cli: the client agent which can perform sending request
//   - bucket: the bucket name
//
// RETURNS:
//   - *GetBucketLifecycleResult: the lifecycle rule of the bucket
//   - error: nil if success otherwise the specific error
func GetBucketLifecycle(cli bce.Client, bucket string, ctx *BosContext,
	options ...Option) (*GetBucketLifecycleResult, error) {
	req := &BosRequest{}
	req.SetUri(getBucketUri(bucket))
	req.SetMethod(http.GET)
	req.SetParam("lifecycle", "")
	req.SetBucket(bucket)
	// handle options to set the header/params of request
	if err := handleOptions(req, options); err != nil {
		return nil, bce.NewBceClientError(fmt.Sprintf("Handle options occur error: %s", err))
	}
	resp := &BosResponse{}
	if err := SendRequest(cli, req, resp, ctx); err != nil {
		return nil, err
	}
	if resp.IsFail() {
		return nil, resp.ServiceError()
	}
	result := &GetBucketLifecycleResult{}
	if err := resp.ParseJsonBody(result); err != nil {
		return nil, err
	}
	return result, nil
}

// DeleteBucketLifecycle - delete the lifecycle rule of the given bucket
//
// PARAMS:
//   - cli: the client agent which can perform sending request
//   - bucket: the bucket name
//
// RETURNS:
//   - error: nil if success otherwise the specific error
func DeleteBucketLifecycle(cli bce.Client, bucket string, ctx *BosContext, options ...Option) error {
	req := &BosRequest{}
	req.SetUri(getBucketUri(bucket))
	req.SetMethod(http.DELETE)
	req.SetParam("lifecycle", "")
	req.SetBucket(bucket)
	// handle options to set the header/params of request
	if err := handleOptions(req, options); err != nil {
		return bce.NewBceClientError(fmt.Sprintf("Handle options occur error: %s", err))
	}
	resp := &BosResponse{}
	if err := SendRequest(cli, req, resp, ctx); err != nil {
		return err
	}
	if resp.IsFail() {
		return resp.ServiceError()
	}
	defer func() { resp.Body().Close() }()
	return nil
}

// PutBucketStorageclass - set the storage class of the given bucket
//
// PARAMS:
//   - cli: the client agent which can perform sending request
//   - bucket: the bucket name
//   - storageClass: the storage class string
//
// RETURNS:
//   - error: nil if success otherwise the specific error
func PutBucketStorageclass(cli bce.Client, bucket, storageClass string, ctx *BosContext, options ...Option) error {
	req := &BosRequest{}
	req.SetUri(getBucketUri(bucket))
	req.SetMethod(http.PUT)
	req.SetParam("storageClass", "")
	req.SetBucket(bucket)
	obj := &StorageClassType{storageClass}
	jsonBytes, jsonErr := json.Marshal(obj)
	if jsonErr != nil {
		return jsonErr
	}
	body, err := bce.NewBodyFromBytes(jsonBytes)
	if err != nil {
		return err
	}
	req.SetBody(body)
	// handle options to set the header/params of request
	if err := handleOptions(req, options); err != nil {
		return bce.NewBceClientError(fmt.Sprintf("Handle options occur error: %s", err))
	}
	resp := &BosResponse{}
	if err := SendRequest(cli, req, resp, ctx); err != nil {
		return err
	}
	if resp.IsFail() {
		return resp.ServiceError()
	}
	defer func() { resp.Body().Close() }()
	return nil
}

// GetBucketStorageclass - get the storage class of the given bucket
//
// PARAMS:
//   - cli: the client agent which can perform sending request
//   - bucket: the bucket name
//
// RETURNS:
//   - string: the storage class of the bucket
//   - error: nil if success otherwise the specific error
func GetBucketStorageclass(cli bce.Client, bucket string, ctx *BosContext, options ...Option) (string, error) {
	req := &BosRequest{}
	req.SetUri(getBucketUri(bucket))
	req.SetMethod(http.GET)
	req.SetParam("storageClass", "")
	req.SetBucket(bucket)
	// handle options to set the header/params of request
	if err := handleOptions(req, options); err != nil {
		return "", bce.NewBceClientError(fmt.Sprintf("Handle options occur error: %s", err))
	}
	resp := &BosResponse{}
	if err := SendRequest(cli, req, resp, ctx); err != nil {
		return "", err
	}
	if resp.IsFail() {
		return "", resp.ServiceError()
	}
	result := &StorageClassType{}
	if err := resp.ParseJsonBody(result); err != nil {
		return "", err
	}
	return result.StorageClass, nil
}

// PutBucketReplication - set the bucket replication of different region
//
// PARAMS:
//   - cli: the client agent which can perform sending request
//   - bucket: the bucket name
//   - replicationConf: the replication config body stream
//   - replicationRuleId: the replication rule id composed of [0-9 A-Z a-z _ -]
//
// RETURNS:
//   - error: nil if success otherwise the specific error
func PutBucketReplication(cli bce.Client, bucket string, replicationConf *bce.Body, replicationRuleId string,
	ctx *BosContext, options ...Option) error {
	req := &BosRequest{}
	req.SetUri(getBucketUri(bucket))
	req.SetMethod(http.PUT)
	req.SetParam("replication", "")
	req.SetBucket(bucket)
	if len(replicationRuleId) > 0 {
		req.SetParam("id", replicationRuleId)
	}

	if replicationConf != nil {
		req.SetHeader(http.CONTENT_TYPE, bce.DEFAULT_CONTENT_TYPE)
		req.SetBody(replicationConf)
	}
	// handle options to set the header/params of request
	if err := handleOptions(req, options); err != nil {
		return bce.NewBceClientError(fmt.Sprintf("Handle options occur error: %s", err))
	}
	resp := &BosResponse{}
	if err := SendRequest(cli, req, resp, ctx); err != nil {
		return err
	}
	if resp.IsFail() {
		return resp.ServiceError()
	}
	defer func() { resp.Body().Close() }()
	return nil
}

// GetBucketReplication - get the bucket replication config of the given bucket
//
// PARAMS:
//   - cli: the client agent which can perform sending request
//   - bucket: the bucket name
//   - replicationRuleId: the replication rule id composed of [0-9 A-Z a-z _ -]
//
// RETURNS:
//   - *GetBucketReplicationResult: the result of the bucket replication config
//   - error: nil if success otherwise the specific error
func GetBucketReplication(cli bce.Client, bucket string, replicationRuleId string,
	ctx *BosContext, options ...Option) (*GetBucketReplicationResult, error) {
	req := &BosRequest{}
	req.SetUri(getBucketUri(bucket))
	req.SetMethod(http.GET)
	req.SetParam("replication", "")
	req.SetBucket(bucket)
	if len(replicationRuleId) > 0 {
		req.SetParam("id", replicationRuleId)
	}
	// handle options to set the header/params of request
	if err := handleOptions(req, options); err != nil {
		return nil, bce.NewBceClientError(fmt.Sprintf("Handle options occur error: %s", err))
	}
	resp := &BosResponse{}
	if err := SendRequest(cli, req, resp, ctx); err != nil {
		return nil, err
	}
	if resp.IsFail() {
		return nil, resp.ServiceError()
	}
	result := &GetBucketReplicationResult{}
	if err := resp.ParseJsonBody(result); err != nil {
		return nil, err
	}
	return result, nil
}

// ListBucketReplication - list all replication config of the given bucket
//
// PARAMS:
//   - cli: the client agent which can perform sending request
//   - bucket: the bucket name
//
// RETURNS:
//   - error: nil if success otherwise the specific error
func ListBucketReplication(cli bce.Client, bucket string, ctx *BosContext,
	options ...Option) (*ListBucketReplicationResult, error) {
	req := &BosRequest{}
	req.SetUri(getBucketUri(bucket))
	req.SetMethod(http.GET)
	req.SetParam("replication", "")
	req.SetParam("list", "")
	req.SetBucket(bucket)
	req.SetContext(ctx.Ctx)
	// handle options to set the header/params of request
	if err := handleOptions(req, options); err != nil {
		return nil, bce.NewBceClientError(fmt.Sprintf("Handle options occur error: %s", err))
	}
	resp := &BosResponse{}
	if err := cli.SendRequest(&req.BceRequest, &resp.BceResponse); err != nil {
		return nil, err
	}
	if resp.IsFail() {
		return nil, resp.ServiceError()
	}
	result := &ListBucketReplicationResult{}
	if err := resp.ParseJsonBody(result); err != nil {
		return nil, err
	}
	return result, nil
}

// DeleteBucketReplication - delete the bucket replication config of the given bucket
//
// PARAMS:
//   - cli: the client agent which can perform sending request
//   - bucket: the bucket name
//   - replicationRuleId: the replication rule id composed of [0-9 A-Z a-z _ -]
//
// RETURNS:
//   - error: nil if success otherwise the specific error
func DeleteBucketReplication(cli bce.Client, bucket string, replicationRuleId string,
	ctx *BosContext, options ...Option) error {
	req := &BosRequest{}
	req.SetUri(getBucketUri(bucket))
	req.SetMethod(http.DELETE)
	req.SetParam("replication", "")
	req.SetBucket(bucket)
	if len(replicationRuleId) > 0 {
		req.SetParam("id", replicationRuleId)
	}
	// handle options to set the header/params of request
	if err := handleOptions(req, options); err != nil {
		return bce.NewBceClientError(fmt.Sprintf("Handle options occur error: %s", err))
	}
	resp := &BosResponse{}
	if err := SendRequest(cli, req, resp, ctx); err != nil {
		return err
	}
	if resp.IsFail() {
		return resp.ServiceError()
	}
	defer func() { resp.Body().Close() }()
	return nil
}

// GetBucketReplicationProgress - get the bucket replication process of the given bucket
//
// PARAMS:
//   - cli: the client agent which can perform sending request
//   - bucket: the bucket name
//   - replicationRuleId: the replication rule id composed of [0-9 A-Z a-z _ -]
//
// RETURNS:
//   - *GetBucketReplicationProgressResult: the result of the bucket replication process
//   - error: nil if success otherwise the specific error
func GetBucketReplicationProgress(cli bce.Client, bucket string, replicationRuleId string,
	ctx *BosContext, options ...Option) (*GetBucketReplicationProgressResult, error) {
	req := &BosRequest{}
	req.SetUri(getBucketUri(bucket))
	req.SetMethod(http.GET)
	req.SetParam("replicationProgress", "")
	req.SetBucket(bucket)
	if len(replicationRuleId) > 0 {
		req.SetParam("id", replicationRuleId)
	}
	// handle options to set the header/params of request
	if err := handleOptions(req, options); err != nil {
		return nil, bce.NewBceClientError(fmt.Sprintf("Handle options occur error: %s", err))
	}
	resp := &BosResponse{}
	if err := SendRequest(cli, req, resp, ctx); err != nil {
		return nil, err
	}
	if resp.IsFail() {
		return nil, resp.ServiceError()
	}
	result := &GetBucketReplicationProgressResult{}
	if err := resp.ParseJsonBody(result); err != nil {
		return nil, err
	}
	return result, nil
}

// PutBucketEncryption - set the bucket encrpytion config
//
// PARAMS:
//   - cli: the client agent which can perform sending request
//   - bucket: the bucket name
//   - algorithm: the encryption algorithm
//
// RETURNS:
//   - error: nil if success otherwise the specific error
func PutBucketEncryption(cli bce.Client, bucket, algorithm string, ctx *BosContext, options ...Option) error {
	req := &BosRequest{}
	req.SetUri(getBucketUri(bucket))
	req.SetMethod(http.PUT)
	req.SetParam("encryption", "")
	req.SetBucket(bucket)
	obj := &BucketEncryptionType{algorithm}
	jsonBytes, jsonErr := json.Marshal(obj)
	if jsonErr != nil {
		return jsonErr
	}
	body, err := bce.NewBodyFromBytes(jsonBytes)
	if err != nil {
		return err
	}
	req.SetHeader(http.CONTENT_TYPE, bce.DEFAULT_CONTENT_TYPE)
	req.SetBody(body)
	// handle options to set the header/params of request
	if err := handleOptions(req, options); err != nil {
		return bce.NewBceClientError(fmt.Sprintf("Handle options occur error: %s", err))
	}
	resp := &BosResponse{}
	if err := SendRequest(cli, req, resp, ctx); err != nil {
		return err
	}
	if resp.IsFail() {
		return resp.ServiceError()
	}
	defer func() { resp.Body().Close() }()
	return nil
}

// GetBucketEncryption - get the encryption config of the given bucket
//
// PARAMS:
//   - cli: the client agent which can perform sending request
//   - bucket: the bucket name
//
// RETURNS:
//   - algorithm: the bucket encryption algorithm
//   - error: nil if success otherwise the specific error
func GetBucketEncryption(cli bce.Client, bucket string, ctx *BosContext, options ...Option) (string, error) {
	req := &BosRequest{}
	req.SetUri(getBucketUri(bucket))
	req.SetMethod(http.GET)
	req.SetParam("encryption", "")
	req.SetBucket(bucket)
	// handle options to set the header/params of request
	if err := handleOptions(req, options); err != nil {
		return "", bce.NewBceClientError(fmt.Sprintf("Handle options occur error: %s", err))
	}
	resp := &BosResponse{}
	if err := SendRequest(cli, req, resp, ctx); err != nil {
		return "", err
	}
	if resp.IsFail() {
		return "", resp.ServiceError()
	}
	result := &BucketEncryptionType{}
	if err := resp.ParseJsonBody(result); err != nil {
		return "", err
	}
	return result.EncryptionAlgorithm, nil
}

// DeleteBucketEncryption - delete the encryption config of the given bucket
//
// PARAMS:
//   - cli: the client agent which can perform sending request
//   - bucket: the bucket name
//
// RETURNS:
//   - error: nil if success otherwise the specific error
func DeleteBucketEncryption(cli bce.Client, bucket string, ctx *BosContext, options ...Option) error {
	req := &BosRequest{}
	req.SetUri(getBucketUri(bucket))
	req.SetMethod(http.DELETE)
	req.SetParam("encryption", "")
	req.SetBucket(bucket)
	// handle options to set the header/params of request
	if err := handleOptions(req, options); err != nil {
		return bce.NewBceClientError(fmt.Sprintf("Handle options occur error: %s", err))
	}
	resp := &BosResponse{}
	if err := SendRequest(cli, req, resp, ctx); err != nil {
		return err
	}
	if resp.IsFail() {
		return resp.ServiceError()
	}
	defer func() { resp.Body().Close() }()
	return nil
}

// PutBucketStaticWebsite - set the bucket static website config
//
// PARAMS:
//   - cli: the client agent which can perform sending request
//   - bucket: the bucket name
//   - confBody: the static website config body stream
//
// RETURNS:
//   - error: nil if success otherwise the specific error
func PutBucketStaticWebsite(cli bce.Client, bucket string, confBody *bce.Body,
	ctx *BosContext, options ...Option) error {
	req := &BosRequest{}
	req.SetUri(getBucketUri(bucket))
	req.SetMethod(http.PUT)
	req.SetParam("website", "")
	req.SetBucket(bucket)
	if confBody != nil {
		req.SetHeader(http.CONTENT_TYPE, bce.DEFAULT_CONTENT_TYPE)
		req.SetBody(confBody)
	}
	// handle options to set the header/params of request
	if err := handleOptions(req, options); err != nil {
		return bce.NewBceClientError(fmt.Sprintf("Handle options occur error: %s", err))
	}
	resp := &BosResponse{}
	if err := SendRequest(cli, req, resp, ctx); err != nil {
		return err
	}
	if resp.IsFail() {
		return resp.ServiceError()
	}
	defer func() { resp.Body().Close() }()
	return nil
}

// GetBucketStaticWebsite - get the static website config of the given bucket
//
// PARAMS:
//   - cli: the client agent which can perform sending request
//   - bucket: the bucket name
//
// RETURNS:
//   - result: the bucket static website config result object
//   - error: nil if success otherwise the specific error
func GetBucketStaticWebsite(cli bce.Client, bucket string, ctx *BosContext,
	options ...Option) (*GetBucketStaticWebsiteResult, error) {
	req := &BosRequest{}
	req.SetUri(getBucketUri(bucket))
	req.SetMethod(http.GET)
	req.SetParam("website", "")
	req.SetBucket(bucket)
	// handle options to set the header/params of request
	if err := handleOptions(req, options); err != nil {
		return nil, bce.NewBceClientError(fmt.Sprintf("Handle options occur error: %s", err))
	}
	resp := &BosResponse{}
	if err := SendRequest(cli, req, resp, ctx); err != nil {
		return nil, err
	}
	if resp.IsFail() {
		return nil, resp.ServiceError()
	}
	result := &GetBucketStaticWebsiteResult{}
	if err := resp.ParseJsonBody(result); err != nil {
		return nil, err
	}
	return result, nil
}

// DeleteBucketStaticWebsite - delete the static website config of the given bucket
//
// PARAMS:
//   - cli: the client agent which can perform sending request
//   - bucket: the bucket name
//
// RETURNS:
//   - error: nil if success otherwise the specific error
func DeleteBucketStaticWebsite(cli bce.Client, bucket string, ctx *BosContext, options ...Option) error {
	req := &BosRequest{}
	req.SetUri(getBucketUri(bucket))
	req.SetMethod(http.DELETE)
	req.SetParam("website", "")
	req.SetBucket(bucket)
	// handle options to set the header/params of request
	if err := handleOptions(req, options); err != nil {
		return bce.NewBceClientError(fmt.Sprintf("Handle options occur error: %s", err))
	}
	resp := &BosResponse{}
	if err := SendRequest(cli, req, resp, ctx); err != nil {
		return err
	}
	if resp.IsFail() {
		return resp.ServiceError()
	}
	defer func() { resp.Body().Close() }()
	return nil
}

// PutBucketCors - set the bucket CORS config
//
// PARAMS:
//   - cli: the client agent which can perform sending request
//   - bucket: the bucket name
//   - confBody: the CORS config body stream
//
// RETURNS:
//   - error: nil if success otherwise the specific error
func PutBucketCors(cli bce.Client, bucket string, confBody *bce.Body, ctx *BosContext, options ...Option) error {
	req := &BosRequest{}
	req.SetUri(getBucketUri(bucket))
	req.SetMethod(http.PUT)
	req.SetParam("cors", "")
	req.SetBucket(bucket)
	if confBody != nil {
		req.SetHeader(http.CONTENT_TYPE, bce.DEFAULT_CONTENT_TYPE)
		req.SetBody(confBody)
	}
	// handle options to set the header/params of request
	if err := handleOptions(req, options); err != nil {
		return bce.NewBceClientError(fmt.Sprintf("Handle options occur error: %s", err))
	}
	resp := &BosResponse{}
	if err := SendRequest(cli, req, resp, ctx); err != nil {
		return err
	}
	if resp.IsFail() {
		return resp.ServiceError()
	}
	defer func() { resp.Body().Close() }()
	return nil
}

// GetBucketCors - get the CORS config of the given bucket
//
// PARAMS:
//   - cli: the client agent which can perform sending request
//   - bucket: the bucket name
//
// RETURNS:
//   - result: the bucket CORS config result object
//   - error: nil if success otherwise the specific error
func GetBucketCors(cli bce.Client, bucket string, ctx *BosContext,
	options ...Option) (*GetBucketCorsResult, error) {
	req := &BosRequest{}
	req.SetUri(getBucketUri(bucket))
	req.SetMethod(http.GET)
	req.SetParam("cors", "")
	req.SetBucket(bucket)
	// handle options to set the header/params of request
	if err := handleOptions(req, options); err != nil {
		return nil, bce.NewBceClientError(fmt.Sprintf("Handle options occur error: %s", err))
	}
	resp := &BosResponse{}
	if err := SendRequest(cli, req, resp, ctx); err != nil {
		return nil, err
	}
	if resp.IsFail() {
		return nil, resp.ServiceError()
	}
	result := &GetBucketCorsResult{}
	if err := resp.ParseJsonBody(result); err != nil {
		return nil, err
	}
	return result, nil
}

// DeleteBucketCors - delete the CORS config of the given bucket
//
// PARAMS:
//   - cli: the client agent which can perform sending request
//   - bucket: the bucket name
//
// RETURNS:
//   - error: nil if success otherwise the specific error
func DeleteBucketCors(cli bce.Client, bucket string, ctx *BosContext, options ...Option) error {
	req := &BosRequest{}
	req.SetUri(getBucketUri(bucket))
	req.SetMethod(http.DELETE)
	req.SetParam("cors", "")
	req.SetBucket(bucket)
	// handle options to set the header/params of request
	if err := handleOptions(req, options); err != nil {
		return bce.NewBceClientError(fmt.Sprintf("Handle options occur error: %s", err))
	}
	resp := &BosResponse{}
	if err := SendRequest(cli, req, resp, ctx); err != nil {
		return err
	}
	if resp.IsFail() {
		return resp.ServiceError()
	}
	defer func() { resp.Body().Close() }()
	return nil
}

// PutBucketCopyrightProtection - set the copyright protection config of the given bucket
//
// PARAMS:
//   - cli: the client agent which can perform sending request
//   - bucket: the bucket name
//   - resources: the resource items in the bucket to be protected
//
// RETURNS:
//   - error: nil if success otherwise the specific error
func PutBucketCopyrightProtection(cli bce.Client, ctx *BosContext, bucket string, resources ...string) error {
	req := &BosRequest{}
	req.SetUri(getBucketUri(bucket))
	req.SetMethod(http.PUT)
	req.SetParam("copyrightProtection", "")
	req.SetBucket(bucket)
	if len(resources) == 0 {
		return bce.NewBceClientError("the resource to set copyright protection is empty")
	}
	arg := &CopyrightProtectionType{resources}
	jsonBytes, jsonErr := json.Marshal(arg)
	if jsonErr != nil {
		return jsonErr
	}
	body, err := bce.NewBodyFromBytes(jsonBytes)
	if err != nil {
		return err
	}
	req.SetHeader(http.CONTENT_TYPE, bce.DEFAULT_CONTENT_TYPE)
	req.SetBody(body)

	resp := &BosResponse{}
	if err := SendRequest(cli, req, resp, ctx); err != nil {
		return err
	}
	if resp.IsFail() {
		return resp.ServiceError()
	}
	defer func() { resp.Body().Close() }()
	return nil
}

// GetBucketCopyrightProtection - get the copyright protection config of the given bucket
//
// PARAMS:
//   - cli: the client agent which can perform sending request
//   - bucket: the bucket name
//
// RETURNS:
//   - result: the bucket copyright protection resources array
//   - error: nil if success otherwise the specific error
func GetBucketCopyrightProtection(cli bce.Client, bucket string,
	ctx *BosContext, options ...Option) ([]string, error) {
	req := &BosRequest{}
	req.SetUri(getBucketUri(bucket))
	req.SetMethod(http.GET)
	req.SetParam("copyrightProtection", "")
	req.SetBucket(bucket)
	// handle options to set the header/params of request
	if err := handleOptions(req, options); err != nil {
		return nil, bce.NewBceClientError(fmt.Sprintf("Handle options occur error: %s", err))
	}
	resp := &BosResponse{}
	if err := SendRequest(cli, req, resp, ctx); err != nil {
		return nil, err
	}
	if resp.IsFail() {
		return nil, resp.ServiceError()
	}
	result := &CopyrightProtectionType{}
	if err := resp.ParseJsonBody(result); err != nil {
		return nil, err
	}
	return result.Resource, nil
}

// DeleteBucketCopyrightProtection - delete the copyright protection config of the given bucket
//
// PARAMS:
//   - cli: the client agent which can perform sending request
//   - bucket: the bucket name
//
// RETURNS:
//   - error: nil if success otherwise the specific error
func DeleteBucketCopyrightProtection(cli bce.Client, bucket string, ctx *BosContext, options ...Option) error {
	req := &BosRequest{}
	req.SetUri(getBucketUri(bucket))
	req.SetMethod(http.DELETE)
	req.SetParam("copyrightProtection", "")
	req.SetBucket(bucket)
	// handle options to set the header/params of request
	if err := handleOptions(req, options); err != nil {
		return bce.NewBceClientError(fmt.Sprintf("Handle options occur error: %s", err))
	}
	resp := &BosResponse{}
	if err := SendRequest(cli, req, resp, ctx); err != nil {
		return err
	}
	if resp.IsFail() {
		return resp.ServiceError()
	}
	defer func() { resp.Body().Close() }()
	return nil
}

// PutBucketTrash - put the trash setting of the given bucket
//
// PARAMS:
//   - cli: the client agent which can perform sending request
//   - bucket: the bucket name
//   - trashDir: the trash dir name
//
// RETURNS:
//   - error: nil if success otherwise the specific error
func PutBucketTrash(cli bce.Client, bucket string, trashReq PutBucketTrashReq,
	ctx *BosContext, options ...Option) error {
	req := &BosRequest{}
	req.SetUri(getBucketUri(bucket))
	req.SetMethod(http.PUT)
	req.SetParam("trash", "")
	req.SetBucket(bucket)
	reqByte, _ := json.Marshal(trashReq)
	body, err := bce.NewBodyFromString(string(reqByte))
	if err != nil {
		return err
	}
	req.SetBody(body)
	// handle options to set the header/params of request
	if err := handleOptions(req, options); err != nil {
		return bce.NewBceClientError(fmt.Sprintf("Handle options occur error: %s", err))
	}
	resp := &BosResponse{}
	if err := SendRequest(cli, req, resp, ctx); err != nil {
		return err
	}
	if resp.IsFail() {
		return resp.ServiceError()
	}
	defer func() { resp.Body().Close() }()
	return nil
}

func GetBucketTrash(cli bce.Client, bucket string, ctx *BosContext,
	options ...Option) (*GetBucketTrashResult, error) {
	req := &BosRequest{}
	req.SetUri(getBucketUri(bucket))
	req.SetMethod(http.GET)
	req.SetParam("trash", "")
	req.SetBucket(bucket)
	// handle options to set the header/params of request
	if err := handleOptions(req, options); err != nil {
		return nil, bce.NewBceClientError(fmt.Sprintf("Handle options occur error: %s", err))
	}
	resp := &BosResponse{}
	if err := SendRequest(cli, req, resp, ctx); err != nil {
		return nil, err
	}
	if resp.IsFail() {
		return nil, resp.ServiceError()
	}
	result := &GetBucketTrashResult{}
	if err := resp.ParseJsonBody(result); err != nil {
		return nil, err
	}
	return result, nil
}

func DeleteBucketTrash(cli bce.Client, bucket string, ctx *BosContext, options ...Option) error {
	req := &BosRequest{}
	req.SetUri(getBucketUri(bucket))
	req.SetMethod(http.DELETE)
	req.SetParam("trash", "")
	req.SetBucket(bucket)
	// handle options to set the header/params of request
	if err := handleOptions(req, options); err != nil {
		return bce.NewBceClientError(fmt.Sprintf("Handle options occur error: %s", err))
	}
	resp := &BosResponse{}
	if err := SendRequest(cli, req, resp, ctx); err != nil {
		return err
	}
	if resp.IsFail() {
		return resp.ServiceError()
	}
	defer func() { resp.Body().Close() }()
	return nil
}

func PutBucketNotification(cli bce.Client, bucket string, putBucketNotificationReq PutBucketNotificationReq,
	ctx *BosContext, options ...Option) error {
	req := &BosRequest{}
	req.SetUri(getBucketUri(bucket))
	req.SetMethod(http.PUT)
	req.SetParam("notification", "")
	req.SetBucket(bucket)
	reqByte, _ := json.Marshal(putBucketNotificationReq)
	body, err := bce.NewBodyFromString(string(reqByte))
	if err != nil {
		return err
	}
	req.SetBody(body)
	// handle options to set the header/params of request
	if err := handleOptions(req, options); err != nil {
		return bce.NewBceClientError(fmt.Sprintf("Handle options occur error: %s", err))
	}
	resp := &BosResponse{}
	if err := SendRequest(cli, req, resp, ctx); err != nil {
		return err
	}
	if resp.IsFail() {
		return resp.ServiceError()
	}
	defer func() { resp.Body().Close() }()
	return nil
}

func GetBucketNotification(cli bce.Client, bucket string, ctx *BosContext,
	options ...Option) (*PutBucketNotificationReq, error) {
	req := &BosRequest{}
	req.SetUri(getBucketUri(bucket))
	req.SetMethod(http.GET)
	req.SetParam("notification", "")
	req.SetBucket(bucket)
	// handle options to set the header/params of request
	if err := handleOptions(req, options); err != nil {
		return nil, bce.NewBceClientError(fmt.Sprintf("Handle options occur error: %s", err))
	}
	resp := &BosResponse{}
	if err := SendRequest(cli, req, resp, ctx); err != nil {
		return nil, err
	}
	if resp.IsFail() {
		return nil, resp.ServiceError()
	}
	result := &PutBucketNotificationReq{}
	if err := resp.ParseJsonBody(result); err != nil {
		return nil, err
	}
	return result, nil
}

func DeleteBucketNotification(cli bce.Client, bucket string, ctx *BosContext, options ...Option) error {
	req := &BosRequest{}
	req.SetUri(getBucketUri(bucket))
	req.SetMethod(http.DELETE)
	req.SetParam("notification", "")
	req.SetBucket(bucket)
	// handle options to set the header/params of request
	if err := handleOptions(req, options); err != nil {
		return bce.NewBceClientError(fmt.Sprintf("Handle options occur error: %s", err))
	}
	resp := &BosResponse{}
	if err := SendRequest(cli, req, resp, ctx); err != nil {
		return err
	}
	if resp.IsFail() {
		return resp.ServiceError()
	}
	defer func() { resp.Body().Close() }()
	return nil
}

func PutBucketMirror(cli bce.Client, bucket string, putBucketMirrorArgs *PutBucketMirrorArgs,
	ctx *BosContext, options ...Option) error {
	req := &BosRequest{}
	req.SetUri(getBucketUri(bucket))
	req.SetMethod(http.PUT)
	req.SetParam("mirroring", "")
	req.SetBucket(bucket)
	reqByte, _ := json.Marshal(putBucketMirrorArgs)
	body, err := bce.NewBodyFromString(string(reqByte))
	if err != nil {
		return err
	}
	req.SetBody(body)
	// handle options to set the header/params of request
	if err := handleOptions(req, options); err != nil {
		return bce.NewBceClientError(fmt.Sprintf("Handle options occur error: %s", err))
	}
	resp := &BosResponse{}
	if err := SendRequest(cli, req, resp, ctx); err != nil {
		return err
	}
	if resp.IsFail() {
		return resp.ServiceError()
	}
	defer func() { resp.Body().Close() }()
	return nil
}

func GetBucketMirror(cli bce.Client, bucket string, ctx *BosContext,
	options ...Option) (*PutBucketMirrorArgs, error) {
	req := &BosRequest{}
	req.SetUri(getBucketUri(bucket))
	req.SetMethod(http.GET)
	req.SetParam("mirroring", "")
	req.SetBucket(bucket)
	// handle options to set the header/params of request
	if err := handleOptions(req, options); err != nil {
		return nil, bce.NewBceClientError(fmt.Sprintf("Handle options occur error: %s", err))
	}
	resp := &BosResponse{}
	if err := SendRequest(cli, req, resp, ctx); err != nil {
		return nil, err
	}
	if resp.IsFail() {
		return nil, resp.ServiceError()
	}
	result := &PutBucketMirrorArgs{}
	if err := resp.ParseJsonBody(result); err != nil {
		return nil, err
	}
	return result, nil
}

func DeleteBucketMirror(cli bce.Client, bucket string, ctx *BosContext, options ...Option) error {
	req := &BosRequest{}
	req.SetUri(getBucketUri(bucket))
	req.SetMethod(http.DELETE)
	req.SetParam("mirroring", "")
	req.SetBucket(bucket)
	// handle options to set the header/params of request
	if err := handleOptions(req, options); err != nil {
		return bce.NewBceClientError(fmt.Sprintf("Handle options occur error: %s", err))
	}
	resp := &BosResponse{}
	if err := SendRequest(cli, req, resp, ctx); err != nil {
		return err
	}
	if resp.IsFail() {
		return resp.ServiceError()
	}
	defer func() { resp.Body().Close() }()
	return nil
}

func PutBucketTag(cli bce.Client, bucket string, putBucketTagArgs *PutBucketTagArgs,
	ctx *BosContext, options ...Option) error {
	req := &BosRequest{}
	req.SetUri(getBucketUri(bucket))
	req.SetMethod(http.PUT)
	req.SetParam("tagging", "")
	req.SetBucket(bucket)
	reqByte, _ := json.Marshal(putBucketTagArgs)
	body, err := bce.NewBodyFromString(string(reqByte))
	if err != nil {
		return err
	}
	req.SetBody(body)
	// handle options to set the header/params of request
	if err := handleOptions(req, options); err != nil {
		return bce.NewBceClientError(fmt.Sprintf("Handle options occur error: %s", err))
	}
	resp := &BosResponse{}
	if err := SendRequest(cli, req, resp, ctx); err != nil {
		return err
	}
	if resp.IsFail() {
		return resp.ServiceError()
	}
	defer func() { resp.Body().Close() }()
	return nil
}

func GetBucketTag(cli bce.Client, bucket string, ctx *BosContext, options ...Option) (*GetBucketTagResult, error) {
	req := &BosRequest{}
	req.SetUri(getBucketUri(bucket))
	req.SetMethod(http.GET)
	req.SetParam("tagging", "")
	req.SetBucket(bucket)
	// handle options to set the header/params of request
	if err := handleOptions(req, options); err != nil {
		return nil, bce.NewBceClientError(fmt.Sprintf("Handle options occur error: %s", err))
	}
	resp := &BosResponse{}
	if err := SendRequest(cli, req, resp, ctx); err != nil {
		return nil, err
	}
	if resp.IsFail() {
		return nil, resp.ServiceError()
	}
	result := &GetBucketTagResult{}
	if err := resp.ParseJsonBody(result); err != nil {
		return nil, err
	}
	return result, nil
}

func DeleteBucketTag(cli bce.Client, bucket string, ctx *BosContext, options ...Option) error {
	req := &BosRequest{}
	req.SetUri(getBucketUri(bucket))
	req.SetMethod(http.DELETE)
	req.SetParam("tagging", "")
	req.SetBucket(bucket)
	// handle options to set the header/params of request
	if err := handleOptions(req, options); err != nil {
		return bce.NewBceClientError(fmt.Sprintf("Handle options occur error: %s", err))
	}
	resp := &BosResponse{}
	if err := SendRequest(cli, req, resp, ctx); err != nil {
		return err
	}
	if resp.IsFail() {
		return resp.ServiceError()
	}
	defer func() { resp.Body().Close() }()
	return nil
}

func GetBosShareLink(cli bce.Client, bucket, prefix, shareCode string, duration int,
	ctx *BosContext, options ...Option) (string, error) {
	req := &BosRequest{}
	req.SetEndpoint(BOS_SHARE_ENDPOINT)
	req.SetParam("action", "")
	req.SetMethod(http.POST)
	req.SetContext(ctx.Ctx)
	if len(shareCode) != 0 && len(shareCode) != 6 {
		return "", fmt.Errorf("shareCode length must be 0 or 6")
	}
	if duration < 60 || duration > 64800 {
		return "", fmt.Errorf("duration must between 1 minute and 18 hours")
	}
	bosShareReqBody := &BosShareLinkArgs{
		Bucket:          bucket,
		Endpoint:        cli.GetBceClientConfig().Endpoint,
		Prefix:          prefix,
		ShareCode:       shareCode,
		DurationSeconds: int64(duration),
	}
	jsonBytes, jsonErr := json.Marshal(bosShareReqBody)
	if jsonErr != nil {
		return "", jsonErr
	}
	body, err := bce.NewBodyFromBytes(jsonBytes)
	if err != nil {
		return "", err
	}
	req.SetBody(body)
	// handle options to set the header/params of request
	if err := handleOptions(req, options); err != nil {
		return "", bce.NewBceClientError(fmt.Sprintf("Handle options occur error: %s", err))
	}
	resp := &BosResponse{}
	if err = cli.SendRequest(&req.BceRequest, &resp.BceResponse); err != nil {
		return "", err
	}
	if resp.IsFail() {
		return "", resp.ServiceError()
	}
	bosShareResBody := &BosShareResBody{}
	if err := resp.ParseJsonBody(bosShareResBody); err != nil {
		return "", err
	}
	jsonData, err := json.Marshal(bosShareResBody)
	if err != nil {
		return "", err
	}
	return string(jsonData), nil
}

func PutBucketVersioning(cli bce.Client, bucket string, putBucketVersioningArgs *BucketVersioningArgs,
	ctx *BosContext, options ...Option) error {
	req := &BosRequest{}
	req.SetUri(getBucketUri(bucket))
	req.SetMethod(http.PUT)
	req.SetParam("versioning", "")
	req.SetBucket(bucket)
	reqByte, _ := json.Marshal(putBucketVersioningArgs)
	body, err := bce.NewBodyFromString(string(reqByte))
	if err != nil {
		return err
	}
	req.SetBody(body)
	// handle options to set the header/params of request
	if err := handleOptions(req, options); err != nil {
		return bce.NewBceClientError(fmt.Sprintf("Handle options occur error: %s", err))
	}
	resp := &BosResponse{}
	if err := SendRequest(cli, req, resp, ctx); err != nil {
		return err
	}
	if resp.IsFail() {
		return resp.ServiceError()
	}
	defer func() { resp.Body().Close() }()
	return nil
}

func GetBucketVersioning(cli bce.Client, bucket string, ctx *BosContext,
	options ...Option) (*BucketVersioningArgs, error) {
	req := &BosRequest{}
	req.SetUri(getBucketUri(bucket))
	req.SetMethod(http.GET)
	req.SetParam("versioning", "")
	req.SetBucket(bucket)
	// handle options to set the header/params of request
	if err := handleOptions(req, options); err != nil {
		return nil, bce.NewBceClientError(fmt.Sprintf("Handle options occur error: %s", err))
	}
	resp := &BosResponse{}
	if err := SendRequest(cli, req, resp, ctx); err != nil {
		return nil, err
	}
	if resp.IsFail() {
		return nil, resp.ServiceError()
	}
	result := &BucketVersioningArgs{}
	if err := resp.ParseJsonBody(result); err != nil {
		return nil, err
	}
	return result, nil
}

// PutBucketInventory - put the inventory config for the given bucket
//
// PARAMS:
//   - cli: the client agent which can perform sending request
//   - bucket: the bucket name
//   - args: inventory configuration
//   - ctx: the context to control the request
//   - options: the function set to set HTTP headers/params
//
// RETURNS:
//   - error: nil if success otherwise the specific error
func PutBucketInventory(cli bce.Client, bucket string, args *PutBucketInventoryArgs,
	ctx *BosContext, options ...Option) error {
	req := &BosRequest{}
	req.SetUri(getBucketUri(bucket))
	req.SetMethod(http.PUT)
	req.SetParam("inventory", "")
	req.SetParam("id", args.Rule.Id)
	req.SetBucket(bucket)
	// handle options to set the header/params of request
	if err := handleOptions(req, options); err != nil {
		return bce.NewBceClientError(fmt.Sprintf("Handle options occur error: %s", err))
	}
	reqByte, _ := json.Marshal(args.Rule)
	body, err := bce.NewBodyFromString(string(reqByte))
	if err != nil {
		return err
	}
	req.SetBody(body)
	resp := &BosResponse{}
	if err := SendRequest(cli, req, resp, ctx); err != nil {
		return err
	}
	if resp.IsFail() {
		return resp.ServiceError()
	}
	defer func() { resp.Body().Close() }()
	return nil
}

// GetBucketInventory - get the inventory config of the given bucket/id
//
// PARAMS:
//   - cli: the client agent which can perform sending request
//   - bucket: the bucket name
//   - id: inventory configuration id
//   - ctx: the context to control the request
//   - options: the function set to set HTTP headers/params
//
// RETURNS:
//   - result: the bucket inventory config result
//   - error: nil if success otherwise the specific error
func GetBucketInventory(cli bce.Client, bucket, id string, ctx *BosContext,
	options ...Option) (*PutBucketInventoryArgs, error) {
	req := &BosRequest{}
	req.SetUri(getBucketUri(bucket))
	req.SetMethod(http.GET)
	req.SetParam("inventory", "")
	req.SetParam("id", id)
	req.SetBucket(bucket)
	// handle options to set the header/params of request
	if err := handleOptions(req, options); err != nil {
		return nil, bce.NewBceClientError(fmt.Sprintf("Handle options occur error: %s", err))
	}
	resp := &BosResponse{}
	if err := SendRequest(cli, req, resp, ctx); err != nil {
		return nil, err
	}
	result := &PutBucketInventoryArgs{}
	if err := resp.ParseJsonBody(&result.Rule); err != nil {
		return nil, err
	}
	return result, nil
}

// ListBucketInventory - list all inventory configs of the given bucket
//
// PARAMS:
//   - cli: the client agent which can perform sending request
//   - bucket: the bucket name
//   - ctx: the context to control the request
//   - options: the function set to set HTTP headers/params
//
// RETURNS:
//   - result: the bucket inventory config result
//   - error: nil if success otherwise the specific error
func ListBucketInventory(cli bce.Client, bucket string, ctx *BosContext,
	options ...Option) (*ListBucketInventoryResult, error) {
	req := &BosRequest{}
	req.SetUri(getBucketUri(bucket))
	req.SetMethod(http.GET)
	req.SetParam("inventory", "")
	req.SetBucket(bucket)
	// handle options to set the header/params of request
	if err := handleOptions(req, options); err != nil {
		return nil, bce.NewBceClientError(fmt.Sprintf("Handle options occur error: %s", err))
	}
	resp := &BosResponse{}
	if err := SendRequest(cli, req, resp, ctx); err != nil {
		return nil, err
	}
	result := &ListBucketInventoryResult{}
	if err := resp.ParseJsonBody(result); err != nil {
		return nil, err
	}
	return result, nil
}

// DeleteBucketInventory - delete the inventory config of the given bucket/id
//
// PARAMS:
//   - cli: the client agent which can perform sending request
//   - bucket: the bucket name
//   - id: inventory configuration id
//   - ctx: the context to control the request
//   - options: the function set to set HTTP headers/params
//
// RETURNS:
//   - error: nil if success otherwise the specific error
func DeleteBucketInventory(cli bce.Client, bucket, id string,
	ctx *BosContext, options ...Option) error {
	req := &BosRequest{}
	req.SetUri(getBucketUri(bucket))
	req.SetMethod(http.DELETE)
	req.SetParam("inventory", "")
	req.SetParam("id", id)
	req.SetBucket(bucket)
	// handle options to set the header/params of request
	if err := handleOptions(req, options); err != nil {
		return bce.NewBceClientError(fmt.Sprintf("Handle options occur error: %s", err))
	}
	resp := &BosResponse{}
	if err := SendRequest(cli, req, resp, ctx); err != nil {
		return err
	}
	if resp.IsFail() {
		return resp.ServiceError()
	}
	return nil
}
