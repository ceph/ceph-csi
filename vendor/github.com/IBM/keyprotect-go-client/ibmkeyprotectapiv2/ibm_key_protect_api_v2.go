/**
 * (C) Copyright IBM Corp. 2026.
 *
 * Licensed under the Apache License, Version 2.0 (the "License");
 * you may not use this file except in compliance with the License.
 * You may obtain a copy of the License at
 *
 *      http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing, software
 * distributed under the License is distributed on an "AS IS" BASIS,
 * WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 * See the License for the specific language governing permissions and
 * limitations under the License.
 */

/*
 * IBM OpenAPI SDK Code Generator Version: 3.115.0-a8d44b59-20260713-123033
 */

// Package ibmkeyprotectapiv2 : Operations and models for the IbmKeyProtectApiV2 service
package ibmkeyprotectapiv2

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"reflect"
	"time"

	"github.com/IBM/go-sdk-core/v5/core"
	"github.com/IBM/keyprotect-go-client/ibmkeyprotectapiv2/common"
	"github.com/go-openapi/strfmt"
)

// IbmKeyProtectApiV2 : IBM Key Protect helps you provision encrypted keys for apps across IBM Cloud. As you manage the
// lifecycle of your keys, you can benefit from knowing that your keys are secured by cloud-based FIPS 140-2 Level 3
// hardware security modules (HSMs) that protect against theft of information. You can use the Key Protect API to store,
// generate, and retrieve your key material. Keys within the service can protect any type of data in your symmetric
// key-based encryption solution.
//
// API Version: 2.0.0
type IbmKeyProtectApiV2 struct {
	Service *core.BaseService
}

// DefaultServiceURL is the default URL to make service requests to.
const DefaultServiceURL = "https://us-south.kms.cloud.ibm.com"

// DefaultServiceName is the default key used to find external configuration information.
const DefaultServiceName = "ibm_key_protect_api"

const ParameterizedServiceURL = "https://{region}.kms.cloud.ibm.com"

var defaultUrlVariables = map[string]string{
	"region": "us-south",
}

// IbmKeyProtectApiV2Options : Service options
type IbmKeyProtectApiV2Options struct {
	ServiceName   string
	URL           string
	Authenticator core.Authenticator
}

// NewIbmKeyProtectApiV2UsingExternalConfig : constructs an instance of IbmKeyProtectApiV2 with passed in options and external configuration.
func NewIbmKeyProtectApiV2UsingExternalConfig(options *IbmKeyProtectApiV2Options) (ibmKeyProtectApi *IbmKeyProtectApiV2, err error) {
	if options.ServiceName == "" {
		options.ServiceName = DefaultServiceName
	}

	if options.Authenticator == nil {
		options.Authenticator, err = core.GetAuthenticatorFromEnvironment(options.ServiceName)
		if err != nil {
			err = core.SDKErrorf(err, "", "env-auth-error", common.GetComponentInfo())
			return
		}
	}

	ibmKeyProtectApi, err = NewIbmKeyProtectApiV2(options)
	err = core.RepurposeSDKProblem(err, "new-client-error")
	if err != nil {
		return
	}

	err = ibmKeyProtectApi.Service.ConfigureService(options.ServiceName)
	if err != nil {
		err = core.SDKErrorf(err, "", "client-config-error", common.GetComponentInfo())
		return
	}

	if options.URL != "" {
		err = ibmKeyProtectApi.Service.SetServiceURL(options.URL)
		err = core.RepurposeSDKProblem(err, "url-set-error")
	}
	return
}

// NewIbmKeyProtectApiV2 : constructs an instance of IbmKeyProtectApiV2 with passed in options.
func NewIbmKeyProtectApiV2(options *IbmKeyProtectApiV2Options) (service *IbmKeyProtectApiV2, err error) {
	serviceOptions := &core.ServiceOptions{
		URL:           DefaultServiceURL,
		Authenticator: options.Authenticator,
	}

	baseService, err := core.NewBaseService(serviceOptions)
	if err != nil {
		err = core.SDKErrorf(err, "", "new-base-error", common.GetComponentInfo())
		return
	}

	if options.URL != "" {
		err = baseService.SetServiceURL(options.URL)
		if err != nil {
			err = core.SDKErrorf(err, "", "set-url-error", common.GetComponentInfo())
			return
		}
	}

	service = &IbmKeyProtectApiV2{
		Service: baseService,
	}

	return
}

// GetServiceURLForRegion returns the service URL to be used for the specified region
func GetServiceURLForRegion(region string) (string, error) {
	return "", core.SDKErrorf(nil, "service does not support regional URLs", "no-regional-support", common.GetComponentInfo())
}

// Clone makes a copy of "ibmKeyProtectApi" suitable for processing requests.
func (ibmKeyProtectApi *IbmKeyProtectApiV2) Clone() *IbmKeyProtectApiV2 {
	if core.IsNil(ibmKeyProtectApi) {
		return nil
	}
	clone := *ibmKeyProtectApi
	clone.Service = ibmKeyProtectApi.Service.Clone()
	return &clone
}

// ConstructServiceURL constructs a service URL from the parameterized URL.
func ConstructServiceURL(providedUrlVariables map[string]string) (string, error) {
	return core.ConstructServiceURL(ParameterizedServiceURL, defaultUrlVariables, providedUrlVariables)
}

// SetServiceURL sets the service URL
func (ibmKeyProtectApi *IbmKeyProtectApiV2) SetServiceURL(url string) error {
	err := ibmKeyProtectApi.Service.SetServiceURL(url)
	if err != nil {
		err = core.SDKErrorf(err, "", "url-set-error", common.GetComponentInfo())
	}
	return err
}

// GetServiceURL returns the service URL
func (ibmKeyProtectApi *IbmKeyProtectApiV2) GetServiceURL() string {
	return ibmKeyProtectApi.Service.GetServiceURL()
}

// SetDefaultHeaders sets HTTP headers to be sent in every request
func (ibmKeyProtectApi *IbmKeyProtectApiV2) SetDefaultHeaders(headers http.Header) {
	ibmKeyProtectApi.Service.SetDefaultHeaders(headers)
}

// SetEnableGzipCompression sets the service's EnableGzipCompression field
func (ibmKeyProtectApi *IbmKeyProtectApiV2) SetEnableGzipCompression(enableGzip bool) {
	ibmKeyProtectApi.Service.SetEnableGzipCompression(enableGzip)
}

// GetEnableGzipCompression returns the service's EnableGzipCompression field
func (ibmKeyProtectApi *IbmKeyProtectApiV2) GetEnableGzipCompression() bool {
	return ibmKeyProtectApi.Service.GetEnableGzipCompression()
}

// EnableRetries enables automatic retries for requests invoked for this service instance.
// If either parameter is specified as 0, then a default value is used instead.
func (ibmKeyProtectApi *IbmKeyProtectApiV2) EnableRetries(maxRetries int, maxRetryInterval time.Duration) {
	ibmKeyProtectApi.Service.EnableRetries(maxRetries, maxRetryInterval)
}

// DisableRetries disables automatic retries for requests invoked for this service instance.
func (ibmKeyProtectApi *IbmKeyProtectApiV2) DisableRetries() {
	ibmKeyProtectApi.Service.DisableRetries()
}

// GetKeyCollectionMetadata : Retrieve key total
// Returns the same HTTP headers as a GET request without returning the entity-body. This operation returns the number
// of keys in your instance in a header called `Key-Total`.
func (ibmKeyProtectApi *IbmKeyProtectApiV2) GetKeyCollectionMetadata(getKeyCollectionMetadataOptions *GetKeyCollectionMetadataOptions) (response *core.DetailedResponse, err error) {
	response, err = ibmKeyProtectApi.GetKeyCollectionMetadataWithContext(context.Background(), getKeyCollectionMetadataOptions)
	err = core.RepurposeSDKProblem(err, "")
	return
}

// GetKeyCollectionMetadataWithContext is an alternate form of the GetKeyCollectionMetadata method which supports a Context parameter
func (ibmKeyProtectApi *IbmKeyProtectApiV2) GetKeyCollectionMetadataWithContext(ctx context.Context, getKeyCollectionMetadataOptions *GetKeyCollectionMetadataOptions) (response *core.DetailedResponse, err error) {
	err = core.ValidateNotNil(getKeyCollectionMetadataOptions, "getKeyCollectionMetadataOptions cannot be nil")
	if err != nil {
		err = core.SDKErrorf(err, "", "unexpected-nil-param", common.GetComponentInfo())
		return
	}
	err = core.ValidateStruct(getKeyCollectionMetadataOptions, "getKeyCollectionMetadataOptions")
	if err != nil {
		err = core.SDKErrorf(err, "", "struct-validation-error", common.GetComponentInfo())
		return
	}

	builder := core.NewRequestBuilder(core.HEAD)
	builder = builder.WithContext(ctx)
	builder.EnableGzipCompression = ibmKeyProtectApi.GetEnableGzipCompression()
	_, err = builder.ResolveRequestURL(ibmKeyProtectApi.Service.Options.URL, `/api/v2/keys`, nil)
	if err != nil {
		err = core.SDKErrorf(err, "", "url-resolve-error", common.GetComponentInfo())
		return
	}

	sdkHeaders := common.GetSdkHeaders("ibm_key_protect_api", "V2", "GetKeyCollectionMetadata")
	for headerName, headerValue := range sdkHeaders {
		builder.AddHeader(headerName, headerValue)
	}

	for headerName, headerValue := range getKeyCollectionMetadataOptions.Headers {
		builder.AddHeader(headerName, headerValue)
	}
	if getKeyCollectionMetadataOptions.BluemixInstance != nil {
		builder.AddHeader("Bluemix-Instance", fmt.Sprint(*getKeyCollectionMetadataOptions.BluemixInstance))
	}
	if getKeyCollectionMetadataOptions.CorrelationID != nil {
		builder.AddHeader("Correlation-Id", fmt.Sprint(*getKeyCollectionMetadataOptions.CorrelationID))
	}
	if getKeyCollectionMetadataOptions.XKmsKeyRing != nil {
		builder.AddHeader("X-Kms-Key-Ring", fmt.Sprint(*getKeyCollectionMetadataOptions.XKmsKeyRing))
	}

	if getKeyCollectionMetadataOptions.State != nil {
		err = builder.AddQuerySlice("state", getKeyCollectionMetadataOptions.State)
		if err != nil {
			err = core.SDKErrorf(err, "", "add-query-slice-error", common.GetComponentInfo())
			return
		}
	}
	if getKeyCollectionMetadataOptions.Extractable != nil {
		builder.AddQuery("extractable", fmt.Sprint(*getKeyCollectionMetadataOptions.Extractable))
	}
	if getKeyCollectionMetadataOptions.Filter != nil {
		builder.AddQuery("filter", fmt.Sprint(*getKeyCollectionMetadataOptions.Filter))
	}

	request, err := builder.Build()
	if err != nil {
		err = core.SDKErrorf(err, "", "build-error", common.GetComponentInfo())
		return
	}

	response, err = ibmKeyProtectApi.Service.Request(request, nil)
	if err != nil {
		core.EnrichHTTPProblem(err, "getKeyCollectionMetadata", getServiceComponentInfo())
		err = core.SDKErrorf(err, "", "http-request-err", common.GetComponentInfo())
		return
	}

	return
}

// CreateKey : Create a key
// Creates a new key with specified key material.
//
// Key Protect designates the resource as either a root key or a standard key based on the `extractable` value that you
// specify. A successful
// `POST /keys` operation adds the key to the service and returns the details of the request in the response
// entity-body, if the Prefer header is set to `return=representation`.
func (ibmKeyProtectApi *IbmKeyProtectApiV2) CreateKey(createKeyOptions *CreateKeyOptions) (result *Key, response *core.DetailedResponse, err error) {
	result, response, err = ibmKeyProtectApi.CreateKeyWithContext(context.Background(), createKeyOptions)
	err = core.RepurposeSDKProblem(err, "")
	return
}

// CreateKeyWithContext is an alternate form of the CreateKey method which supports a Context parameter
func (ibmKeyProtectApi *IbmKeyProtectApiV2) CreateKeyWithContext(ctx context.Context, createKeyOptions *CreateKeyOptions) (result *Key, response *core.DetailedResponse, err error) {
	err = core.ValidateNotNil(createKeyOptions, "createKeyOptions cannot be nil")
	if err != nil {
		err = core.SDKErrorf(err, "", "unexpected-nil-param", common.GetComponentInfo())
		return
	}
	err = core.ValidateStruct(createKeyOptions, "createKeyOptions")
	if err != nil {
		err = core.SDKErrorf(err, "", "struct-validation-error", common.GetComponentInfo())
		return
	}

	builder := core.NewRequestBuilder(core.POST)
	builder = builder.WithContext(ctx)
	builder.EnableGzipCompression = ibmKeyProtectApi.GetEnableGzipCompression()
	_, err = builder.ResolveRequestURL(ibmKeyProtectApi.Service.Options.URL, `/api/v2/keys`, nil)
	if err != nil {
		err = core.SDKErrorf(err, "", "url-resolve-error", common.GetComponentInfo())
		return
	}

	sdkHeaders := common.GetSdkHeaders("ibm_key_protect_api", "V2", "CreateKey")
	for headerName, headerValue := range sdkHeaders {
		builder.AddHeader(headerName, headerValue)
	}

	for headerName, headerValue := range createKeyOptions.Headers {
		builder.AddHeader(headerName, headerValue)
	}
	builder.AddHeader("Accept", "application/json")
	builder.AddHeader("Content-Type", "application/vnd.ibm.kms.key+json")
	if createKeyOptions.BluemixInstance != nil {
		builder.AddHeader("Bluemix-Instance", fmt.Sprint(*createKeyOptions.BluemixInstance))
	}
	if createKeyOptions.CorrelationID != nil {
		builder.AddHeader("Correlation-Id", fmt.Sprint(*createKeyOptions.CorrelationID))
	}
	if createKeyOptions.Prefer != nil {
		builder.AddHeader("Prefer", fmt.Sprint(*createKeyOptions.Prefer))
	}
	if createKeyOptions.XKmsKeyRing != nil {
		builder.AddHeader("X-Kms-Key-Ring", fmt.Sprint(*createKeyOptions.XKmsKeyRing))
	}

	_, err = builder.SetBodyContent("application/vnd.ibm.kms.key+json", nil, nil, createKeyOptions.KeyCreateBody)
	if err != nil {
		err = core.SDKErrorf(err, "", "set-body-error", common.GetComponentInfo())
		return
	}

	request, err := builder.Build()
	if err != nil {
		err = core.SDKErrorf(err, "", "build-error", common.GetComponentInfo())
		return
	}

	var rawResponse map[string]json.RawMessage
	response, err = ibmKeyProtectApi.Service.Request(request, &rawResponse)
	if err != nil {
		core.EnrichHTTPProblem(err, "createKey", getServiceComponentInfo())
		err = core.SDKErrorf(err, "", "http-request-err", common.GetComponentInfo())
		return
	}
	if rawResponse != nil {
		err = core.UnmarshalModel(rawResponse, "", &result, UnmarshalKey)
		if err != nil {
			err = core.SDKErrorf(err, "", "unmarshal-resp-error", common.GetComponentInfo())
			return
		}
		response.Result = result
	}

	return
}

// GetKeys : List keys
// Retrieves a list of keys that are stored in your Key Protect service instance.
//
// **Important:** When a user of Key Protect on Satellite views lists of keys through the [IBM
// Console](https://cloud.ibm.com/login), or programmatically via this API, keys with ["fine grain"
// permissions](/docs/key-protect?topic=key-protect-grant-access-keys#grant-access-key-level) won't appear due to the
// manner in which the service aggregates the collection. While the user can still use the key resource, only by using
// the CLI or API and passing the specific key ID can a user access the metadata and other details of the key.
//
// **Note:** `GET /keys` will not return the key material in the response body. You can retrieve the key material for a
// standard key with a subsequent `GET /keys/{id}` request.
func (ibmKeyProtectApi *IbmKeyProtectApiV2) GetKeys(getKeysOptions *GetKeysOptions) (result *ListKeys, response *core.DetailedResponse, err error) {
	result, response, err = ibmKeyProtectApi.GetKeysWithContext(context.Background(), getKeysOptions)
	err = core.RepurposeSDKProblem(err, "")
	return
}

// GetKeysWithContext is an alternate form of the GetKeys method which supports a Context parameter
func (ibmKeyProtectApi *IbmKeyProtectApiV2) GetKeysWithContext(ctx context.Context, getKeysOptions *GetKeysOptions) (result *ListKeys, response *core.DetailedResponse, err error) {
	err = core.ValidateNotNil(getKeysOptions, "getKeysOptions cannot be nil")
	if err != nil {
		err = core.SDKErrorf(err, "", "unexpected-nil-param", common.GetComponentInfo())
		return
	}
	err = core.ValidateStruct(getKeysOptions, "getKeysOptions")
	if err != nil {
		err = core.SDKErrorf(err, "", "struct-validation-error", common.GetComponentInfo())
		return
	}

	builder := core.NewRequestBuilder(core.GET)
	builder = builder.WithContext(ctx)
	builder.EnableGzipCompression = ibmKeyProtectApi.GetEnableGzipCompression()
	_, err = builder.ResolveRequestURL(ibmKeyProtectApi.Service.Options.URL, `/api/v2/keys`, nil)
	if err != nil {
		err = core.SDKErrorf(err, "", "url-resolve-error", common.GetComponentInfo())
		return
	}

	sdkHeaders := common.GetSdkHeaders("ibm_key_protect_api", "V2", "GetKeys")
	for headerName, headerValue := range sdkHeaders {
		builder.AddHeader(headerName, headerValue)
	}

	for headerName, headerValue := range getKeysOptions.Headers {
		builder.AddHeader(headerName, headerValue)
	}
	builder.AddHeader("Accept", "application/json")
	if getKeysOptions.BluemixInstance != nil {
		builder.AddHeader("Bluemix-Instance", fmt.Sprint(*getKeysOptions.BluemixInstance))
	}
	if getKeysOptions.CorrelationID != nil {
		builder.AddHeader("Correlation-Id", fmt.Sprint(*getKeysOptions.CorrelationID))
	}
	if getKeysOptions.XKmsKeyRing != nil {
		builder.AddHeader("X-Kms-Key-Ring", fmt.Sprint(*getKeysOptions.XKmsKeyRing))
	}

	if getKeysOptions.Limit != nil {
		builder.AddQuery("limit", fmt.Sprint(*getKeysOptions.Limit))
	}
	if getKeysOptions.Offset != nil {
		builder.AddQuery("offset", fmt.Sprint(*getKeysOptions.Offset))
	}
	if getKeysOptions.State != nil {
		err = builder.AddQuerySlice("state", getKeysOptions.State)
		if err != nil {
			err = core.SDKErrorf(err, "", "add-query-slice-error", common.GetComponentInfo())
			return
		}
	}
	if getKeysOptions.Extractable != nil {
		builder.AddQuery("extractable", fmt.Sprint(*getKeysOptions.Extractable))
	}
	if getKeysOptions.Search != nil {
		builder.AddQuery("search", fmt.Sprint(*getKeysOptions.Search))
	}
	if getKeysOptions.Sort != nil {
		builder.AddQuery("sort", fmt.Sprint(*getKeysOptions.Sort))
	}
	if getKeysOptions.Filter != nil {
		builder.AddQuery("filter", fmt.Sprint(*getKeysOptions.Filter))
	}

	request, err := builder.Build()
	if err != nil {
		err = core.SDKErrorf(err, "", "build-error", common.GetComponentInfo())
		return
	}

	var rawResponse map[string]json.RawMessage
	response, err = ibmKeyProtectApi.Service.Request(request, &rawResponse)
	if err != nil {
		core.EnrichHTTPProblem(err, "getKeys", getServiceComponentInfo())
		err = core.SDKErrorf(err, "", "http-request-err", common.GetComponentInfo())
		return
	}
	if rawResponse != nil {
		err = core.UnmarshalModel(rawResponse, "", &result, UnmarshalListKeys)
		if err != nil {
			err = core.SDKErrorf(err, "", "unmarshal-resp-error", common.GetComponentInfo())
			return
		}
		response.Result = result
	}

	return
}

// CreateKeyWithPoliciesOverrides : Create a key with policy overrides
// Creates a new key with specified key material and key policies. This API overrides the policy configurations set at
// instance level with policies provided in the payload. Key Protect designates the resource as a root key or a standard
// key based on the extractable value that you specify. A successful `POST /keys_with_policy_overrides` operation adds
// the key and key policies to the service and returns the details of the request in the response entity-body, if the
// Prefer header is set to `return=representation`.
func (ibmKeyProtectApi *IbmKeyProtectApiV2) CreateKeyWithPoliciesOverrides(createKeyWithPoliciesOverridesOptions *CreateKeyWithPoliciesOverridesOptions) (result *Key, response *core.DetailedResponse, err error) {
	result, response, err = ibmKeyProtectApi.CreateKeyWithPoliciesOverridesWithContext(context.Background(), createKeyWithPoliciesOverridesOptions)
	err = core.RepurposeSDKProblem(err, "")
	return
}

// CreateKeyWithPoliciesOverridesWithContext is an alternate form of the CreateKeyWithPoliciesOverrides method which supports a Context parameter
func (ibmKeyProtectApi *IbmKeyProtectApiV2) CreateKeyWithPoliciesOverridesWithContext(ctx context.Context, createKeyWithPoliciesOverridesOptions *CreateKeyWithPoliciesOverridesOptions) (result *Key, response *core.DetailedResponse, err error) {
	err = core.ValidateNotNil(createKeyWithPoliciesOverridesOptions, "createKeyWithPoliciesOverridesOptions cannot be nil")
	if err != nil {
		err = core.SDKErrorf(err, "", "unexpected-nil-param", common.GetComponentInfo())
		return
	}
	err = core.ValidateStruct(createKeyWithPoliciesOverridesOptions, "createKeyWithPoliciesOverridesOptions")
	if err != nil {
		err = core.SDKErrorf(err, "", "struct-validation-error", common.GetComponentInfo())
		return
	}

	builder := core.NewRequestBuilder(core.POST)
	builder = builder.WithContext(ctx)
	builder.EnableGzipCompression = ibmKeyProtectApi.GetEnableGzipCompression()
	_, err = builder.ResolveRequestURL(ibmKeyProtectApi.Service.Options.URL, `/api/v2/keys_with_policy_overrides`, nil)
	if err != nil {
		err = core.SDKErrorf(err, "", "url-resolve-error", common.GetComponentInfo())
		return
	}

	sdkHeaders := common.GetSdkHeaders("ibm_key_protect_api", "V2", "CreateKeyWithPoliciesOverrides")
	for headerName, headerValue := range sdkHeaders {
		builder.AddHeader(headerName, headerValue)
	}

	for headerName, headerValue := range createKeyWithPoliciesOverridesOptions.Headers {
		builder.AddHeader(headerName, headerValue)
	}
	builder.AddHeader("Accept", "application/json")
	builder.AddHeader("Content-Type", "application/vnd.ibm.kms.key+json")
	if createKeyWithPoliciesOverridesOptions.BluemixInstance != nil {
		builder.AddHeader("Bluemix-Instance", fmt.Sprint(*createKeyWithPoliciesOverridesOptions.BluemixInstance))
	}
	if createKeyWithPoliciesOverridesOptions.CorrelationID != nil {
		builder.AddHeader("Correlation-Id", fmt.Sprint(*createKeyWithPoliciesOverridesOptions.CorrelationID))
	}
	if createKeyWithPoliciesOverridesOptions.Prefer != nil {
		builder.AddHeader("Prefer", fmt.Sprint(*createKeyWithPoliciesOverridesOptions.Prefer))
	}
	if createKeyWithPoliciesOverridesOptions.XKmsKeyRing != nil {
		builder.AddHeader("X-Kms-Key-Ring", fmt.Sprint(*createKeyWithPoliciesOverridesOptions.XKmsKeyRing))
	}

	_, err = builder.SetBodyContent("application/vnd.ibm.kms.key+json", nil, nil, createKeyWithPoliciesOverridesOptions.KeyWithPolicyOverridesCreateBody)
	if err != nil {
		err = core.SDKErrorf(err, "", "set-body-error", common.GetComponentInfo())
		return
	}

	request, err := builder.Build()
	if err != nil {
		err = core.SDKErrorf(err, "", "build-error", common.GetComponentInfo())
		return
	}

	var rawResponse map[string]json.RawMessage
	response, err = ibmKeyProtectApi.Service.Request(request, &rawResponse)
	if err != nil {
		core.EnrichHTTPProblem(err, "createKeyWithPoliciesOverrides", getServiceComponentInfo())
		err = core.SDKErrorf(err, "", "http-request-err", common.GetComponentInfo())
		return
	}
	if rawResponse != nil {
		err = core.UnmarshalModel(rawResponse, "", &result, UnmarshalKey)
		if err != nil {
			err = core.SDKErrorf(err, "", "unmarshal-resp-error", common.GetComponentInfo())
			return
		}
		response.Result = result
	}

	return
}

// GetKey : Retrieve a key
// Retrieves a key and its details by specifying the ID or alias of the key.
func (ibmKeyProtectApi *IbmKeyProtectApiV2) GetKey(getKeyOptions *GetKeyOptions) (result *GetKey, response *core.DetailedResponse, err error) {
	result, response, err = ibmKeyProtectApi.GetKeyWithContext(context.Background(), getKeyOptions)
	err = core.RepurposeSDKProblem(err, "")
	return
}

// GetKeyWithContext is an alternate form of the GetKey method which supports a Context parameter
func (ibmKeyProtectApi *IbmKeyProtectApiV2) GetKeyWithContext(ctx context.Context, getKeyOptions *GetKeyOptions) (result *GetKey, response *core.DetailedResponse, err error) {
	err = core.ValidateNotNil(getKeyOptions, "getKeyOptions cannot be nil")
	if err != nil {
		err = core.SDKErrorf(err, "", "unexpected-nil-param", common.GetComponentInfo())
		return
	}
	err = core.ValidateStruct(getKeyOptions, "getKeyOptions")
	if err != nil {
		err = core.SDKErrorf(err, "", "struct-validation-error", common.GetComponentInfo())
		return
	}

	pathParamsMap := map[string]string{
		"id": *getKeyOptions.ID,
	}

	builder := core.NewRequestBuilder(core.GET)
	builder = builder.WithContext(ctx)
	builder.EnableGzipCompression = ibmKeyProtectApi.GetEnableGzipCompression()
	_, err = builder.ResolveRequestURL(ibmKeyProtectApi.Service.Options.URL, `/api/v2/keys/{id}`, pathParamsMap)
	if err != nil {
		err = core.SDKErrorf(err, "", "url-resolve-error", common.GetComponentInfo())
		return
	}

	sdkHeaders := common.GetSdkHeaders("ibm_key_protect_api", "V2", "GetKey")
	for headerName, headerValue := range sdkHeaders {
		builder.AddHeader(headerName, headerValue)
	}

	for headerName, headerValue := range getKeyOptions.Headers {
		builder.AddHeader(headerName, headerValue)
	}
	builder.AddHeader("Accept", "application/json")
	if getKeyOptions.BluemixInstance != nil {
		builder.AddHeader("Bluemix-Instance", fmt.Sprint(*getKeyOptions.BluemixInstance))
	}
	if getKeyOptions.CorrelationID != nil {
		builder.AddHeader("Correlation-Id", fmt.Sprint(*getKeyOptions.CorrelationID))
	}
	if getKeyOptions.XKmsKeyRing != nil {
		builder.AddHeader("X-Kms-Key-Ring", fmt.Sprint(*getKeyOptions.XKmsKeyRing))
	}

	request, err := builder.Build()
	if err != nil {
		err = core.SDKErrorf(err, "", "build-error", common.GetComponentInfo())
		return
	}

	var rawResponse map[string]json.RawMessage
	response, err = ibmKeyProtectApi.Service.Request(request, &rawResponse)
	if err != nil {
		core.EnrichHTTPProblem(err, "getKey", getServiceComponentInfo())
		err = core.SDKErrorf(err, "", "http-request-err", common.GetComponentInfo())
		return
	}
	if rawResponse != nil {
		err = core.UnmarshalModel(rawResponse, "", &result, UnmarshalGetKey)
		if err != nil {
			err = core.SDKErrorf(err, "", "unmarshal-resp-error", common.GetComponentInfo())
			return
		}
		response.Result = result
	}

	return
}

// ActionOnKey : Invoke an action on a key
// **Note:** This API has been **deprecated** and transitioned to individual request paths. Existing actions using this
// API will continue to be supported, but new actions will no longer be added to it. We recommend, if possible, aligning
// your request URLs to the new API path. The generic format of actions is now the following:
// `/api/v2/keys/<key_ID>/actions/<action>` where `key_ID` is the key you want to operate on/with and `action` is the
// same action that was passed as a query parameter previously.
//
// Invokes an action on a specified key. This method supports the following actions:
//
// - `disable`: [Disable operations](/docs/key-protect?topic=key-protect-disable-keys) for a key
// - `enable`: [Enable operations](/docs/key-protect?topic=key-protect-disable-keys#enable-api) for a key
// - `restore`: [Restore a root key](/docs/key-protect?topic=key-protect-restore-keys)
// - `rewrap`: Use a root key to [rewrap or reencrypt a data encryption
// key](/docs/key-protect?topic=key-protect-rewrap-keys)
// - `rotate`: [Create a new version](/docs/key-protect?topic=key-protect-rotate-keys) of a root key
// - `setKeyForDeletion`: [Authorize
// deletion](/docs/key-protect?topic=key-protect-delete-dual-auth-keys#set-key-deletion-api) for a key with a dual
// authorization policy
// - `unsetKeyForDeletion`: [Remove an
// authorization](/docs/key-protect?topic=key-protect-delete-dual-auth-keys#unset-key-deletion-api) for a key with a
// dual authorization policy
// - `unwrap`: Use a root key to [unwrap or decrypt a data encryption
// key](/docs/key-protect?topic=key-protect-unwrap-keys)
// - `wrap`: Use a root key to [wrap or encrypt a data encryption key](/docs/key-protect?topic=key-protect-wrap-keys)
//
// **Note:** If you unwrap a wrapped data encryption key (WDEK) that was not wrapped by the latest version of the key,
// the service also returns the a new WDEK, wrapped with the latest version of the key as the ciphertext field. The
// recommendation is to store and use that WDEK, although older WDEKs will continue to work.
// Deprecated: this method is deprecated and may be removed in a future release.
func (ibmKeyProtectApi *IbmKeyProtectApiV2) ActionOnKey(actionOnKeyOptions *ActionOnKeyOptions) (result KeyActionOneOfResponseIntf, response *core.DetailedResponse, err error) {
	result, response, err = ibmKeyProtectApi.ActionOnKeyWithContext(context.Background(), actionOnKeyOptions)
	err = core.RepurposeSDKProblem(err, "")
	return
}

// ActionOnKeyWithContext is an alternate form of the ActionOnKey method which supports a Context parameter
// Deprecated: this method is deprecated and may be removed in a future release.
func (ibmKeyProtectApi *IbmKeyProtectApiV2) ActionOnKeyWithContext(ctx context.Context, actionOnKeyOptions *ActionOnKeyOptions) (result KeyActionOneOfResponseIntf, response *core.DetailedResponse, err error) {
	core.GetLogger().Warn("A deprecated operation has been invoked: ActionOnKey")
	err = core.ValidateNotNil(actionOnKeyOptions, "actionOnKeyOptions cannot be nil")
	if err != nil {
		err = core.SDKErrorf(err, "", "unexpected-nil-param", common.GetComponentInfo())
		return
	}
	err = core.ValidateStruct(actionOnKeyOptions, "actionOnKeyOptions")
	if err != nil {
		err = core.SDKErrorf(err, "", "struct-validation-error", common.GetComponentInfo())
		return
	}

	pathParamsMap := map[string]string{
		"id": *actionOnKeyOptions.ID,
	}

	builder := core.NewRequestBuilder(core.POST)
	builder = builder.WithContext(ctx)
	builder.EnableGzipCompression = ibmKeyProtectApi.GetEnableGzipCompression()
	_, err = builder.ResolveRequestURL(ibmKeyProtectApi.Service.Options.URL, `/api/v2/keys/{id}`, pathParamsMap)
	if err != nil {
		err = core.SDKErrorf(err, "", "url-resolve-error", common.GetComponentInfo())
		return
	}

	sdkHeaders := common.GetSdkHeaders("ibm_key_protect_api", "V2", "ActionOnKey")
	for headerName, headerValue := range sdkHeaders {
		builder.AddHeader(headerName, headerValue)
	}

	for headerName, headerValue := range actionOnKeyOptions.Headers {
		builder.AddHeader(headerName, headerValue)
	}
	builder.AddHeader("Accept", "application/json")
	builder.AddHeader("Content-Type", "application/vnd.ibm.kms.key_action+json")
	if actionOnKeyOptions.BluemixInstance != nil {
		builder.AddHeader("Bluemix-Instance", fmt.Sprint(*actionOnKeyOptions.BluemixInstance))
	}
	if actionOnKeyOptions.CorrelationID != nil {
		builder.AddHeader("Correlation-Id", fmt.Sprint(*actionOnKeyOptions.CorrelationID))
	}
	if actionOnKeyOptions.XKmsKeyRing != nil {
		builder.AddHeader("X-Kms-Key-Ring", fmt.Sprint(*actionOnKeyOptions.XKmsKeyRing))
	}
	if actionOnKeyOptions.Prefer != nil {
		builder.AddHeader("Prefer", fmt.Sprint(*actionOnKeyOptions.Prefer))
	}

	builder.AddQuery("action", fmt.Sprint(*actionOnKeyOptions.Action))

	_, err = builder.SetBodyContent("application/vnd.ibm.kms.key_action+json", nil, nil, actionOnKeyOptions.KeyActionBody)
	if err != nil {
		err = core.SDKErrorf(err, "", "set-body-error", common.GetComponentInfo())
		return
	}

	request, err := builder.Build()
	if err != nil {
		err = core.SDKErrorf(err, "", "build-error", common.GetComponentInfo())
		return
	}

	var rawResponse map[string]json.RawMessage
	response, err = ibmKeyProtectApi.Service.Request(request, &rawResponse)
	if err != nil {
		core.EnrichHTTPProblem(err, "actionOnKey", getServiceComponentInfo())
		err = core.SDKErrorf(err, "", "http-request-err", common.GetComponentInfo())
		return
	}
	if rawResponse != nil {
		err = core.UnmarshalModel(rawResponse, "", &result, UnmarshalKeyActionOneOfResponse)
		if err != nil {
			err = core.SDKErrorf(err, "", "unmarshal-resp-error", common.GetComponentInfo())
			return
		}
		response.Result = result
	}

	return
}

// PatchKey : Update (patch) a key
// Update attributes of a key. Currently only the following attributes are applicable for update: - keyRingID Note: If
// provided, the `X-Kms-Key-Ring` header should specify the key's current key ring. To change the key ring of the key,
// specify the new key ring in the request body.
func (ibmKeyProtectApi *IbmKeyProtectApiV2) PatchKey(patchKeyOptions *PatchKeyOptions) (result *PatchKeyResponseBody, response *core.DetailedResponse, err error) {
	result, response, err = ibmKeyProtectApi.PatchKeyWithContext(context.Background(), patchKeyOptions)
	err = core.RepurposeSDKProblem(err, "")
	return
}

// PatchKeyWithContext is an alternate form of the PatchKey method which supports a Context parameter
func (ibmKeyProtectApi *IbmKeyProtectApiV2) PatchKeyWithContext(ctx context.Context, patchKeyOptions *PatchKeyOptions) (result *PatchKeyResponseBody, response *core.DetailedResponse, err error) {
	err = core.ValidateNotNil(patchKeyOptions, "patchKeyOptions cannot be nil")
	if err != nil {
		err = core.SDKErrorf(err, "", "unexpected-nil-param", common.GetComponentInfo())
		return
	}
	err = core.ValidateStruct(patchKeyOptions, "patchKeyOptions")
	if err != nil {
		err = core.SDKErrorf(err, "", "struct-validation-error", common.GetComponentInfo())
		return
	}

	pathParamsMap := map[string]string{
		"id": *patchKeyOptions.ID,
	}

	builder := core.NewRequestBuilder(core.PATCH)
	builder = builder.WithContext(ctx)
	builder.EnableGzipCompression = ibmKeyProtectApi.GetEnableGzipCompression()
	_, err = builder.ResolveRequestURL(ibmKeyProtectApi.Service.Options.URL, `/api/v2/keys/{id}`, pathParamsMap)
	if err != nil {
		err = core.SDKErrorf(err, "", "url-resolve-error", common.GetComponentInfo())
		return
	}

	sdkHeaders := common.GetSdkHeaders("ibm_key_protect_api", "V2", "PatchKey")
	for headerName, headerValue := range sdkHeaders {
		builder.AddHeader(headerName, headerValue)
	}

	for headerName, headerValue := range patchKeyOptions.Headers {
		builder.AddHeader(headerName, headerValue)
	}
	builder.AddHeader("Accept", "application/json")
	builder.AddHeader("Content-Type", "application/vnd.ibm.kms.key+json")
	if patchKeyOptions.BluemixInstance != nil {
		builder.AddHeader("Bluemix-Instance", fmt.Sprint(*patchKeyOptions.BluemixInstance))
	}
	if patchKeyOptions.CorrelationID != nil {
		builder.AddHeader("Correlation-Id", fmt.Sprint(*patchKeyOptions.CorrelationID))
	}
	if patchKeyOptions.XKmsKeyRing != nil {
		builder.AddHeader("X-Kms-Key-Ring", fmt.Sprint(*patchKeyOptions.XKmsKeyRing))
	}

	_, err = builder.SetBodyContent("application/vnd.ibm.kms.key+json", nil, nil, patchKeyOptions.KeyPatchBody)
	if err != nil {
		err = core.SDKErrorf(err, "", "set-body-error", common.GetComponentInfo())
		return
	}

	request, err := builder.Build()
	if err != nil {
		err = core.SDKErrorf(err, "", "build-error", common.GetComponentInfo())
		return
	}

	var rawResponse map[string]json.RawMessage
	response, err = ibmKeyProtectApi.Service.Request(request, &rawResponse)
	if err != nil {
		core.EnrichHTTPProblem(err, "patchKey", getServiceComponentInfo())
		err = core.SDKErrorf(err, "", "http-request-err", common.GetComponentInfo())
		return
	}
	if rawResponse != nil {
		err = core.UnmarshalModel(rawResponse, "", &result, UnmarshalPatchKeyResponseBody)
		if err != nil {
			err = core.SDKErrorf(err, "", "unmarshal-resp-error", common.GetComponentInfo())
			return
		}
		response.Result = result
	}

	return
}

// DeleteKey : Delete a key
// Deletes a key by specifying the ID or alias of the key.
//
// By default, Key Protect requires a single authorization to delete keys. For added protection, you can
// [enable a dual authorization policy](#set-key-policies) to safely delete keys from your service instance.
//
// **Important:** After a key has been deleted, any data that is encrypted by the key becomes inaccessible, though this
// can be reversed if the key is restored within the 30-day time frame. After 30 days, key metadata, registrations, and
// policies are available for up to 90 days, at which point the key becomes eligible to be purged. Note that once a key
// is no longer restorable and has been purged, its associated data can no longer be accessed.
//
// **Note:** By default, Key Protect blocks the deletion of a key that's protecting a cloud resource, such as a Cloud
// Object Storage bucket. Use
// `GET keys/{id}/registrations` to verify if the key has an active registration to a resource. To delete the key and
// its associated registrations, set the optional `force` parameter to `true`.
func (ibmKeyProtectApi *IbmKeyProtectApiV2) DeleteKey(deleteKeyOptions *DeleteKeyOptions) (result *DeleteKey, response *core.DetailedResponse, err error) {
	result, response, err = ibmKeyProtectApi.DeleteKeyWithContext(context.Background(), deleteKeyOptions)
	err = core.RepurposeSDKProblem(err, "")
	return
}

// DeleteKeyWithContext is an alternate form of the DeleteKey method which supports a Context parameter
func (ibmKeyProtectApi *IbmKeyProtectApiV2) DeleteKeyWithContext(ctx context.Context, deleteKeyOptions *DeleteKeyOptions) (result *DeleteKey, response *core.DetailedResponse, err error) {
	err = core.ValidateNotNil(deleteKeyOptions, "deleteKeyOptions cannot be nil")
	if err != nil {
		err = core.SDKErrorf(err, "", "unexpected-nil-param", common.GetComponentInfo())
		return
	}
	err = core.ValidateStruct(deleteKeyOptions, "deleteKeyOptions")
	if err != nil {
		err = core.SDKErrorf(err, "", "struct-validation-error", common.GetComponentInfo())
		return
	}

	pathParamsMap := map[string]string{
		"id": *deleteKeyOptions.ID,
	}

	builder := core.NewRequestBuilder(core.DELETE)
	builder = builder.WithContext(ctx)
	builder.EnableGzipCompression = ibmKeyProtectApi.GetEnableGzipCompression()
	_, err = builder.ResolveRequestURL(ibmKeyProtectApi.Service.Options.URL, `/api/v2/keys/{id}`, pathParamsMap)
	if err != nil {
		err = core.SDKErrorf(err, "", "url-resolve-error", common.GetComponentInfo())
		return
	}

	sdkHeaders := common.GetSdkHeaders("ibm_key_protect_api", "V2", "DeleteKey")
	for headerName, headerValue := range sdkHeaders {
		builder.AddHeader(headerName, headerValue)
	}

	for headerName, headerValue := range deleteKeyOptions.Headers {
		builder.AddHeader(headerName, headerValue)
	}
	builder.AddHeader("Accept", "application/json")
	if deleteKeyOptions.BluemixInstance != nil {
		builder.AddHeader("Bluemix-Instance", fmt.Sprint(*deleteKeyOptions.BluemixInstance))
	}
	if deleteKeyOptions.CorrelationID != nil {
		builder.AddHeader("Correlation-Id", fmt.Sprint(*deleteKeyOptions.CorrelationID))
	}
	if deleteKeyOptions.XKmsKeyRing != nil {
		builder.AddHeader("X-Kms-Key-Ring", fmt.Sprint(*deleteKeyOptions.XKmsKeyRing))
	}
	if deleteKeyOptions.Prefer != nil {
		builder.AddHeader("Prefer", fmt.Sprint(*deleteKeyOptions.Prefer))
	}

	if deleteKeyOptions.Force != nil {
		builder.AddQuery("force", fmt.Sprint(*deleteKeyOptions.Force))
	}

	request, err := builder.Build()
	if err != nil {
		err = core.SDKErrorf(err, "", "build-error", common.GetComponentInfo())
		return
	}

	var rawResponse map[string]json.RawMessage
	response, err = ibmKeyProtectApi.Service.Request(request, &rawResponse)
	if err != nil {
		core.EnrichHTTPProblem(err, "deleteKey", getServiceComponentInfo())
		err = core.SDKErrorf(err, "", "http-request-err", common.GetComponentInfo())
		return
	}
	if rawResponse != nil {
		err = core.UnmarshalModel(rawResponse, "", &result, UnmarshalDeleteKey)
		if err != nil {
			err = core.SDKErrorf(err, "", "unmarshal-resp-error", common.GetComponentInfo())
			return
		}
		response.Result = result
	}

	return
}

// GetKeyMetadata : Retrieve key metadata
// Retrieves the details of a key by specifying the ID of the key.
func (ibmKeyProtectApi *IbmKeyProtectApiV2) GetKeyMetadata(getKeyMetadataOptions *GetKeyMetadataOptions) (result *GetKeyMetadata, response *core.DetailedResponse, err error) {
	result, response, err = ibmKeyProtectApi.GetKeyMetadataWithContext(context.Background(), getKeyMetadataOptions)
	err = core.RepurposeSDKProblem(err, "")
	return
}

// GetKeyMetadataWithContext is an alternate form of the GetKeyMetadata method which supports a Context parameter
func (ibmKeyProtectApi *IbmKeyProtectApiV2) GetKeyMetadataWithContext(ctx context.Context, getKeyMetadataOptions *GetKeyMetadataOptions) (result *GetKeyMetadata, response *core.DetailedResponse, err error) {
	err = core.ValidateNotNil(getKeyMetadataOptions, "getKeyMetadataOptions cannot be nil")
	if err != nil {
		err = core.SDKErrorf(err, "", "unexpected-nil-param", common.GetComponentInfo())
		return
	}
	err = core.ValidateStruct(getKeyMetadataOptions, "getKeyMetadataOptions")
	if err != nil {
		err = core.SDKErrorf(err, "", "struct-validation-error", common.GetComponentInfo())
		return
	}

	pathParamsMap := map[string]string{
		"id": *getKeyMetadataOptions.ID,
	}

	builder := core.NewRequestBuilder(core.GET)
	builder = builder.WithContext(ctx)
	builder.EnableGzipCompression = ibmKeyProtectApi.GetEnableGzipCompression()
	_, err = builder.ResolveRequestURL(ibmKeyProtectApi.Service.Options.URL, `/api/v2/keys/{id}/metadata`, pathParamsMap)
	if err != nil {
		err = core.SDKErrorf(err, "", "url-resolve-error", common.GetComponentInfo())
		return
	}

	sdkHeaders := common.GetSdkHeaders("ibm_key_protect_api", "V2", "GetKeyMetadata")
	for headerName, headerValue := range sdkHeaders {
		builder.AddHeader(headerName, headerValue)
	}

	for headerName, headerValue := range getKeyMetadataOptions.Headers {
		builder.AddHeader(headerName, headerValue)
	}
	builder.AddHeader("Accept", "application/json")
	if getKeyMetadataOptions.BluemixInstance != nil {
		builder.AddHeader("Bluemix-Instance", fmt.Sprint(*getKeyMetadataOptions.BluemixInstance))
	}
	if getKeyMetadataOptions.CorrelationID != nil {
		builder.AddHeader("Correlation-Id", fmt.Sprint(*getKeyMetadataOptions.CorrelationID))
	}
	if getKeyMetadataOptions.XKmsKeyRing != nil {
		builder.AddHeader("X-Kms-Key-Ring", fmt.Sprint(*getKeyMetadataOptions.XKmsKeyRing))
	}

	request, err := builder.Build()
	if err != nil {
		err = core.SDKErrorf(err, "", "build-error", common.GetComponentInfo())
		return
	}

	var rawResponse map[string]json.RawMessage
	response, err = ibmKeyProtectApi.Service.Request(request, &rawResponse)
	if err != nil {
		core.EnrichHTTPProblem(err, "getKeyMetadata", getServiceComponentInfo())
		err = core.SDKErrorf(err, "", "http-request-err", common.GetComponentInfo())
		return
	}
	if rawResponse != nil {
		err = core.UnmarshalModel(rawResponse, "", &result, UnmarshalGetKeyMetadata)
		if err != nil {
			err = core.SDKErrorf(err, "", "unmarshal-resp-error", common.GetComponentInfo())
			return
		}
		response.Result = result
	}

	return
}

// PurgeKey : Purge a deleted key
// Purges all key metadata and registrations associated with the specified key. This method requires setting the
// [_KeyPurge_
// permission](https://cloud.ibm.com/docs/key-protect?topic=key-protect-grant-access-keys#grant-access-keys-specific-functions)
// that is not enabled by default. Purging a key can only be applied to a key in the **Destroyed** (5) state. After a
// key is deleted, there is a wait period of up to four hours before purge key operation is allowed.
// **Important:** When you purge a key, you permanently shred its contents and associated data. The action cannot be
// reversed.
func (ibmKeyProtectApi *IbmKeyProtectApiV2) PurgeKey(purgeKeyOptions *PurgeKeyOptions) (result *PurgeKey, response *core.DetailedResponse, err error) {
	result, response, err = ibmKeyProtectApi.PurgeKeyWithContext(context.Background(), purgeKeyOptions)
	err = core.RepurposeSDKProblem(err, "")
	return
}

// PurgeKeyWithContext is an alternate form of the PurgeKey method which supports a Context parameter
func (ibmKeyProtectApi *IbmKeyProtectApiV2) PurgeKeyWithContext(ctx context.Context, purgeKeyOptions *PurgeKeyOptions) (result *PurgeKey, response *core.DetailedResponse, err error) {
	err = core.ValidateNotNil(purgeKeyOptions, "purgeKeyOptions cannot be nil")
	if err != nil {
		err = core.SDKErrorf(err, "", "unexpected-nil-param", common.GetComponentInfo())
		return
	}
	err = core.ValidateStruct(purgeKeyOptions, "purgeKeyOptions")
	if err != nil {
		err = core.SDKErrorf(err, "", "struct-validation-error", common.GetComponentInfo())
		return
	}

	pathParamsMap := map[string]string{
		"id": *purgeKeyOptions.ID,
	}

	builder := core.NewRequestBuilder(core.DELETE)
	builder = builder.WithContext(ctx)
	builder.EnableGzipCompression = ibmKeyProtectApi.GetEnableGzipCompression()
	_, err = builder.ResolveRequestURL(ibmKeyProtectApi.Service.Options.URL, `/api/v2/keys/{id}/purge`, pathParamsMap)
	if err != nil {
		err = core.SDKErrorf(err, "", "url-resolve-error", common.GetComponentInfo())
		return
	}

	sdkHeaders := common.GetSdkHeaders("ibm_key_protect_api", "V2", "PurgeKey")
	for headerName, headerValue := range sdkHeaders {
		builder.AddHeader(headerName, headerValue)
	}

	for headerName, headerValue := range purgeKeyOptions.Headers {
		builder.AddHeader(headerName, headerValue)
	}
	builder.AddHeader("Accept", "application/json")
	if purgeKeyOptions.BluemixInstance != nil {
		builder.AddHeader("Bluemix-Instance", fmt.Sprint(*purgeKeyOptions.BluemixInstance))
	}
	if purgeKeyOptions.CorrelationID != nil {
		builder.AddHeader("Correlation-Id", fmt.Sprint(*purgeKeyOptions.CorrelationID))
	}
	if purgeKeyOptions.XKmsKeyRing != nil {
		builder.AddHeader("X-Kms-Key-Ring", fmt.Sprint(*purgeKeyOptions.XKmsKeyRing))
	}
	if purgeKeyOptions.Prefer != nil {
		builder.AddHeader("Prefer", fmt.Sprint(*purgeKeyOptions.Prefer))
	}

	request, err := builder.Build()
	if err != nil {
		err = core.SDKErrorf(err, "", "build-error", common.GetComponentInfo())
		return
	}

	var rawResponse map[string]json.RawMessage
	response, err = ibmKeyProtectApi.Service.Request(request, &rawResponse)
	if err != nil {
		core.EnrichHTTPProblem(err, "purgeKey", getServiceComponentInfo())
		err = core.SDKErrorf(err, "", "http-request-err", common.GetComponentInfo())
		return
	}
	if rawResponse != nil {
		err = core.UnmarshalModel(rawResponse, "", &result, UnmarshalPurgeKey)
		if err != nil {
			err = core.SDKErrorf(err, "", "unmarshal-resp-error", common.GetComponentInfo())
			return
		}
		response.Result = result
	}

	return
}

// RestoreKey : Restore a key
// [Restore a key](/docs/key-protect?topic=key-protect-restore-keys) that has been deleted.
func (ibmKeyProtectApi *IbmKeyProtectApiV2) RestoreKey(restoreKeyOptions *RestoreKeyOptions) (result io.ReadCloser, response *core.DetailedResponse, err error) {
	result, response, err = ibmKeyProtectApi.RestoreKeyWithContext(context.Background(), restoreKeyOptions)
	err = core.RepurposeSDKProblem(err, "")
	return
}

// RestoreKeyWithContext is an alternate form of the RestoreKey method which supports a Context parameter
func (ibmKeyProtectApi *IbmKeyProtectApiV2) RestoreKeyWithContext(ctx context.Context, restoreKeyOptions *RestoreKeyOptions) (result io.ReadCloser, response *core.DetailedResponse, err error) {
	err = core.ValidateNotNil(restoreKeyOptions, "restoreKeyOptions cannot be nil")
	if err != nil {
		err = core.SDKErrorf(err, "", "unexpected-nil-param", common.GetComponentInfo())
		return
	}
	err = core.ValidateStruct(restoreKeyOptions, "restoreKeyOptions")
	if err != nil {
		err = core.SDKErrorf(err, "", "struct-validation-error", common.GetComponentInfo())
		return
	}

	pathParamsMap := map[string]string{
		"id": *restoreKeyOptions.ID,
	}

	builder := core.NewRequestBuilder(core.POST)
	builder = builder.WithContext(ctx)
	builder.EnableGzipCompression = ibmKeyProtectApi.GetEnableGzipCompression()
	_, err = builder.ResolveRequestURL(ibmKeyProtectApi.Service.Options.URL, `/api/v2/keys/{id}/restore`, pathParamsMap)
	if err != nil {
		err = core.SDKErrorf(err, "", "url-resolve-error", common.GetComponentInfo())
		return
	}

	sdkHeaders := common.GetSdkHeaders("ibm_key_protect_api", "V2", "RestoreKey")
	for headerName, headerValue := range sdkHeaders {
		builder.AddHeader(headerName, headerValue)
	}

	for headerName, headerValue := range restoreKeyOptions.Headers {
		builder.AddHeader(headerName, headerValue)
	}
	builder.AddHeader("Accept", "application/vnd.ibm.kms.key+json")
	if restoreKeyOptions.BluemixInstance != nil {
		builder.AddHeader("Bluemix-Instance", fmt.Sprint(*restoreKeyOptions.BluemixInstance))
	}
	if restoreKeyOptions.CorrelationID != nil {
		builder.AddHeader("Correlation-Id", fmt.Sprint(*restoreKeyOptions.CorrelationID))
	}
	if restoreKeyOptions.XKmsKeyRing != nil {
		builder.AddHeader("X-Kms-Key-Ring", fmt.Sprint(*restoreKeyOptions.XKmsKeyRing))
	}
	if restoreKeyOptions.Prefer != nil {
		builder.AddHeader("Prefer", fmt.Sprint(*restoreKeyOptions.Prefer))
	}

	request, err := builder.Build()
	if err != nil {
		err = core.SDKErrorf(err, "", "build-error", common.GetComponentInfo())
		return
	}

	response, err = ibmKeyProtectApi.Service.Request(request, &result)
	if err != nil {
		core.EnrichHTTPProblem(err, "restoreKey", getServiceComponentInfo())
		err = core.SDKErrorf(err, "", "http-request-err", common.GetComponentInfo())
		return
	}

	return
}

// GetKeyVersions : List key versions
// Retrieves all versions of a root key by specifying the ID or alias of the key.
//
// When you rotate a root key, you generate a new version of the key. If you're using the root key to protect resources
// across IBM Cloud, the registered cloud services that you associate with the key use the latest key version to wrap
// your data.
// [Learn more](/docs/key-protect?topic=key-protect-key-rotation).
func (ibmKeyProtectApi *IbmKeyProtectApiV2) GetKeyVersions(getKeyVersionsOptions *GetKeyVersionsOptions) (result *ListKeyVersions, response *core.DetailedResponse, err error) {
	result, response, err = ibmKeyProtectApi.GetKeyVersionsWithContext(context.Background(), getKeyVersionsOptions)
	err = core.RepurposeSDKProblem(err, "")
	return
}

// GetKeyVersionsWithContext is an alternate form of the GetKeyVersions method which supports a Context parameter
func (ibmKeyProtectApi *IbmKeyProtectApiV2) GetKeyVersionsWithContext(ctx context.Context, getKeyVersionsOptions *GetKeyVersionsOptions) (result *ListKeyVersions, response *core.DetailedResponse, err error) {
	err = core.ValidateNotNil(getKeyVersionsOptions, "getKeyVersionsOptions cannot be nil")
	if err != nil {
		err = core.SDKErrorf(err, "", "unexpected-nil-param", common.GetComponentInfo())
		return
	}
	err = core.ValidateStruct(getKeyVersionsOptions, "getKeyVersionsOptions")
	if err != nil {
		err = core.SDKErrorf(err, "", "struct-validation-error", common.GetComponentInfo())
		return
	}

	pathParamsMap := map[string]string{
		"id": *getKeyVersionsOptions.ID,
	}

	builder := core.NewRequestBuilder(core.GET)
	builder = builder.WithContext(ctx)
	builder.EnableGzipCompression = ibmKeyProtectApi.GetEnableGzipCompression()
	_, err = builder.ResolveRequestURL(ibmKeyProtectApi.Service.Options.URL, `/api/v2/keys/{id}/versions`, pathParamsMap)
	if err != nil {
		err = core.SDKErrorf(err, "", "url-resolve-error", common.GetComponentInfo())
		return
	}

	sdkHeaders := common.GetSdkHeaders("ibm_key_protect_api", "V2", "GetKeyVersions")
	for headerName, headerValue := range sdkHeaders {
		builder.AddHeader(headerName, headerValue)
	}

	for headerName, headerValue := range getKeyVersionsOptions.Headers {
		builder.AddHeader(headerName, headerValue)
	}
	builder.AddHeader("Accept", "application/json")
	if getKeyVersionsOptions.BluemixInstance != nil {
		builder.AddHeader("Bluemix-Instance", fmt.Sprint(*getKeyVersionsOptions.BluemixInstance))
	}
	if getKeyVersionsOptions.CorrelationID != nil {
		builder.AddHeader("Correlation-Id", fmt.Sprint(*getKeyVersionsOptions.CorrelationID))
	}
	if getKeyVersionsOptions.XKmsKeyRing != nil {
		builder.AddHeader("X-Kms-Key-Ring", fmt.Sprint(*getKeyVersionsOptions.XKmsKeyRing))
	}

	if getKeyVersionsOptions.Limit != nil {
		builder.AddQuery("limit", fmt.Sprint(*getKeyVersionsOptions.Limit))
	}
	if getKeyVersionsOptions.Offset != nil {
		builder.AddQuery("offset", fmt.Sprint(*getKeyVersionsOptions.Offset))
	}
	if getKeyVersionsOptions.TotalCount != nil {
		builder.AddQuery("totalCount", fmt.Sprint(*getKeyVersionsOptions.TotalCount))
	}
	if getKeyVersionsOptions.AllKeyStates != nil {
		builder.AddQuery("allKeyStates", fmt.Sprint(*getKeyVersionsOptions.AllKeyStates))
	}

	request, err := builder.Build()
	if err != nil {
		err = core.SDKErrorf(err, "", "build-error", common.GetComponentInfo())
		return
	}

	var rawResponse map[string]json.RawMessage
	response, err = ibmKeyProtectApi.Service.Request(request, &rawResponse)
	if err != nil {
		core.EnrichHTTPProblem(err, "getKeyVersions", getServiceComponentInfo())
		err = core.SDKErrorf(err, "", "http-request-err", common.GetComponentInfo())
		return
	}
	if rawResponse != nil {
		err = core.UnmarshalModel(rawResponse, "", &result, UnmarshalListKeyVersions)
		if err != nil {
			err = core.SDKErrorf(err, "", "unmarshal-resp-error", common.GetComponentInfo())
			return
		}
		response.Result = result
	}

	return
}

// WrapKey : Wrap a key
// Use a root key to [wrap or encrypt a data encryption key](/docs/key-protect?topic=key-protect-wrap-keys). When
// present, the ciphertext contains the DEK wrapped by the latest version of the key (WDEK). It is recommended to store
// and use this WDEK in future calls to Key Protect.
func (ibmKeyProtectApi *IbmKeyProtectApiV2) WrapKey(wrapKeyOptions *WrapKeyOptions) (result *WrapKeyResponseBody, response *core.DetailedResponse, err error) {
	result, response, err = ibmKeyProtectApi.WrapKeyWithContext(context.Background(), wrapKeyOptions)
	err = core.RepurposeSDKProblem(err, "")
	return
}

// WrapKeyWithContext is an alternate form of the WrapKey method which supports a Context parameter
func (ibmKeyProtectApi *IbmKeyProtectApiV2) WrapKeyWithContext(ctx context.Context, wrapKeyOptions *WrapKeyOptions) (result *WrapKeyResponseBody, response *core.DetailedResponse, err error) {
	err = core.ValidateNotNil(wrapKeyOptions, "wrapKeyOptions cannot be nil")
	if err != nil {
		err = core.SDKErrorf(err, "", "unexpected-nil-param", common.GetComponentInfo())
		return
	}
	err = core.ValidateStruct(wrapKeyOptions, "wrapKeyOptions")
	if err != nil {
		err = core.SDKErrorf(err, "", "struct-validation-error", common.GetComponentInfo())
		return
	}

	pathParamsMap := map[string]string{
		"id": *wrapKeyOptions.ID,
	}

	builder := core.NewRequestBuilder(core.POST)
	builder = builder.WithContext(ctx)
	builder.EnableGzipCompression = ibmKeyProtectApi.GetEnableGzipCompression()
	_, err = builder.ResolveRequestURL(ibmKeyProtectApi.Service.Options.URL, `/api/v2/keys/{id}/actions/wrap`, pathParamsMap)
	if err != nil {
		err = core.SDKErrorf(err, "", "url-resolve-error", common.GetComponentInfo())
		return
	}

	sdkHeaders := common.GetSdkHeaders("ibm_key_protect_api", "V2", "WrapKey")
	for headerName, headerValue := range sdkHeaders {
		builder.AddHeader(headerName, headerValue)
	}

	for headerName, headerValue := range wrapKeyOptions.Headers {
		builder.AddHeader(headerName, headerValue)
	}
	builder.AddHeader("Accept", "application/json")
	builder.AddHeader("Content-Type", "application/vnd.ibm.kms.key_action_wrap+json")
	if wrapKeyOptions.BluemixInstance != nil {
		builder.AddHeader("Bluemix-Instance", fmt.Sprint(*wrapKeyOptions.BluemixInstance))
	}
	if wrapKeyOptions.CorrelationID != nil {
		builder.AddHeader("Correlation-Id", fmt.Sprint(*wrapKeyOptions.CorrelationID))
	}
	if wrapKeyOptions.XKmsKeyRing != nil {
		builder.AddHeader("X-Kms-Key-Ring", fmt.Sprint(*wrapKeyOptions.XKmsKeyRing))
	}

	_, err = builder.SetBodyContent("application/vnd.ibm.kms.key_action_wrap+json", nil, nil, wrapKeyOptions.KeyActionWrapBody)
	if err != nil {
		err = core.SDKErrorf(err, "", "set-body-error", common.GetComponentInfo())
		return
	}

	request, err := builder.Build()
	if err != nil {
		err = core.SDKErrorf(err, "", "build-error", common.GetComponentInfo())
		return
	}

	var rawResponse map[string]json.RawMessage
	response, err = ibmKeyProtectApi.Service.Request(request, &rawResponse)
	if err != nil {
		core.EnrichHTTPProblem(err, "wrapKey", getServiceComponentInfo())
		err = core.SDKErrorf(err, "", "http-request-err", common.GetComponentInfo())
		return
	}
	if rawResponse != nil {
		err = core.UnmarshalModel(rawResponse, "", &result, UnmarshalWrapKeyResponseBody)
		if err != nil {
			err = core.SDKErrorf(err, "", "unmarshal-resp-error", common.GetComponentInfo())
			return
		}
		response.Result = result
	}

	return
}

// UnwrapKey : Unwrap a key
// Use a root key to
// [unwrap or decrypt a data encryption key](/docs/key-protect?topic=key-protect-unwrap-keys).
//
// **Note:** When you unwrap a wrapped data encryption key (WDEK) by using a rotated root key, the service returns a new
// ciphertext in the response entity-body. Each ciphertext remains available for `unwrap` actions. If you unwrap a DEK
// with a previous ciphertext, the service also returns the latest ciphertext and latest key version in the response.
// Use the latest ciphertext for future unwrap operations.
func (ibmKeyProtectApi *IbmKeyProtectApiV2) UnwrapKey(unwrapKeyOptions *UnwrapKeyOptions) (result *UnwrapKeyResponseBody, response *core.DetailedResponse, err error) {
	result, response, err = ibmKeyProtectApi.UnwrapKeyWithContext(context.Background(), unwrapKeyOptions)
	err = core.RepurposeSDKProblem(err, "")
	return
}

// UnwrapKeyWithContext is an alternate form of the UnwrapKey method which supports a Context parameter
func (ibmKeyProtectApi *IbmKeyProtectApiV2) UnwrapKeyWithContext(ctx context.Context, unwrapKeyOptions *UnwrapKeyOptions) (result *UnwrapKeyResponseBody, response *core.DetailedResponse, err error) {
	err = core.ValidateNotNil(unwrapKeyOptions, "unwrapKeyOptions cannot be nil")
	if err != nil {
		err = core.SDKErrorf(err, "", "unexpected-nil-param", common.GetComponentInfo())
		return
	}
	err = core.ValidateStruct(unwrapKeyOptions, "unwrapKeyOptions")
	if err != nil {
		err = core.SDKErrorf(err, "", "struct-validation-error", common.GetComponentInfo())
		return
	}

	pathParamsMap := map[string]string{
		"id": *unwrapKeyOptions.ID,
	}

	builder := core.NewRequestBuilder(core.POST)
	builder = builder.WithContext(ctx)
	builder.EnableGzipCompression = ibmKeyProtectApi.GetEnableGzipCompression()
	_, err = builder.ResolveRequestURL(ibmKeyProtectApi.Service.Options.URL, `/api/v2/keys/{id}/actions/unwrap`, pathParamsMap)
	if err != nil {
		err = core.SDKErrorf(err, "", "url-resolve-error", common.GetComponentInfo())
		return
	}

	sdkHeaders := common.GetSdkHeaders("ibm_key_protect_api", "V2", "UnwrapKey")
	for headerName, headerValue := range sdkHeaders {
		builder.AddHeader(headerName, headerValue)
	}

	for headerName, headerValue := range unwrapKeyOptions.Headers {
		builder.AddHeader(headerName, headerValue)
	}
	builder.AddHeader("Accept", "application/json")
	builder.AddHeader("Content-Type", "application/vnd.ibm.kms.key_action_unwrap+json")
	if unwrapKeyOptions.BluemixInstance != nil {
		builder.AddHeader("Bluemix-Instance", fmt.Sprint(*unwrapKeyOptions.BluemixInstance))
	}
	if unwrapKeyOptions.CorrelationID != nil {
		builder.AddHeader("Correlation-Id", fmt.Sprint(*unwrapKeyOptions.CorrelationID))
	}
	if unwrapKeyOptions.XKmsKeyRing != nil {
		builder.AddHeader("X-Kms-Key-Ring", fmt.Sprint(*unwrapKeyOptions.XKmsKeyRing))
	}

	_, err = builder.SetBodyContent("application/vnd.ibm.kms.key_action_unwrap+json", nil, nil, unwrapKeyOptions.KeyActionUnwrapBody)
	if err != nil {
		err = core.SDKErrorf(err, "", "set-body-error", common.GetComponentInfo())
		return
	}

	request, err := builder.Build()
	if err != nil {
		err = core.SDKErrorf(err, "", "build-error", common.GetComponentInfo())
		return
	}

	var rawResponse map[string]json.RawMessage
	response, err = ibmKeyProtectApi.Service.Request(request, &rawResponse)
	if err != nil {
		core.EnrichHTTPProblem(err, "unwrapKey", getServiceComponentInfo())
		err = core.SDKErrorf(err, "", "http-request-err", common.GetComponentInfo())
		return
	}
	if rawResponse != nil {
		err = core.UnmarshalModel(rawResponse, "", &result, UnmarshalUnwrapKeyResponseBody)
		if err != nil {
			err = core.SDKErrorf(err, "", "unmarshal-resp-error", common.GetComponentInfo())
			return
		}
		response.Result = result
	}

	return
}

// RewrapKey : Rewrap a key
// Use a root key to [rewrap or reencrypt a data encryption key](/docs/key-protect?topic=key-protect-rewrap-keys).
func (ibmKeyProtectApi *IbmKeyProtectApiV2) RewrapKey(rewrapKeyOptions *RewrapKeyOptions) (result *RewrapKeyResponseBody, response *core.DetailedResponse, err error) {
	result, response, err = ibmKeyProtectApi.RewrapKeyWithContext(context.Background(), rewrapKeyOptions)
	err = core.RepurposeSDKProblem(err, "")
	return
}

// RewrapKeyWithContext is an alternate form of the RewrapKey method which supports a Context parameter
func (ibmKeyProtectApi *IbmKeyProtectApiV2) RewrapKeyWithContext(ctx context.Context, rewrapKeyOptions *RewrapKeyOptions) (result *RewrapKeyResponseBody, response *core.DetailedResponse, err error) {
	err = core.ValidateNotNil(rewrapKeyOptions, "rewrapKeyOptions cannot be nil")
	if err != nil {
		err = core.SDKErrorf(err, "", "unexpected-nil-param", common.GetComponentInfo())
		return
	}
	err = core.ValidateStruct(rewrapKeyOptions, "rewrapKeyOptions")
	if err != nil {
		err = core.SDKErrorf(err, "", "struct-validation-error", common.GetComponentInfo())
		return
	}

	pathParamsMap := map[string]string{
		"id": *rewrapKeyOptions.ID,
	}

	builder := core.NewRequestBuilder(core.POST)
	builder = builder.WithContext(ctx)
	builder.EnableGzipCompression = ibmKeyProtectApi.GetEnableGzipCompression()
	_, err = builder.ResolveRequestURL(ibmKeyProtectApi.Service.Options.URL, `/api/v2/keys/{id}/actions/rewrap`, pathParamsMap)
	if err != nil {
		err = core.SDKErrorf(err, "", "url-resolve-error", common.GetComponentInfo())
		return
	}

	sdkHeaders := common.GetSdkHeaders("ibm_key_protect_api", "V2", "RewrapKey")
	for headerName, headerValue := range sdkHeaders {
		builder.AddHeader(headerName, headerValue)
	}

	for headerName, headerValue := range rewrapKeyOptions.Headers {
		builder.AddHeader(headerName, headerValue)
	}
	builder.AddHeader("Accept", "application/json")
	builder.AddHeader("Content-Type", "application/vnd.ibm.kms.key_action_rewrap+json")
	if rewrapKeyOptions.BluemixInstance != nil {
		builder.AddHeader("Bluemix-Instance", fmt.Sprint(*rewrapKeyOptions.BluemixInstance))
	}
	if rewrapKeyOptions.CorrelationID != nil {
		builder.AddHeader("Correlation-Id", fmt.Sprint(*rewrapKeyOptions.CorrelationID))
	}
	if rewrapKeyOptions.XKmsKeyRing != nil {
		builder.AddHeader("X-Kms-Key-Ring", fmt.Sprint(*rewrapKeyOptions.XKmsKeyRing))
	}

	_, err = builder.SetBodyContent("application/vnd.ibm.kms.key_action_rewrap+json", nil, nil, rewrapKeyOptions.KeyActionRewrapBody)
	if err != nil {
		err = core.SDKErrorf(err, "", "set-body-error", common.GetComponentInfo())
		return
	}

	request, err := builder.Build()
	if err != nil {
		err = core.SDKErrorf(err, "", "build-error", common.GetComponentInfo())
		return
	}

	var rawResponse map[string]json.RawMessage
	response, err = ibmKeyProtectApi.Service.Request(request, &rawResponse)
	if err != nil {
		core.EnrichHTTPProblem(err, "rewrapKey", getServiceComponentInfo())
		err = core.SDKErrorf(err, "", "http-request-err", common.GetComponentInfo())
		return
	}
	if rawResponse != nil {
		err = core.UnmarshalModel(rawResponse, "", &result, UnmarshalRewrapKeyResponseBody)
		if err != nil {
			err = core.SDKErrorf(err, "", "unmarshal-resp-error", common.GetComponentInfo())
			return
		}
		response.Result = result
	}

	return
}

// RotateKey : Rotate a key
// [Create a new version](/docs/key-protect?topic=key-protect-rotate-keys) of a root key.
func (ibmKeyProtectApi *IbmKeyProtectApiV2) RotateKey(rotateKeyOptions *RotateKeyOptions) (response *core.DetailedResponse, err error) {
	response, err = ibmKeyProtectApi.RotateKeyWithContext(context.Background(), rotateKeyOptions)
	err = core.RepurposeSDKProblem(err, "")
	return
}

// RotateKeyWithContext is an alternate form of the RotateKey method which supports a Context parameter
func (ibmKeyProtectApi *IbmKeyProtectApiV2) RotateKeyWithContext(ctx context.Context, rotateKeyOptions *RotateKeyOptions) (response *core.DetailedResponse, err error) {
	err = core.ValidateNotNil(rotateKeyOptions, "rotateKeyOptions cannot be nil")
	if err != nil {
		err = core.SDKErrorf(err, "", "unexpected-nil-param", common.GetComponentInfo())
		return
	}
	err = core.ValidateStruct(rotateKeyOptions, "rotateKeyOptions")
	if err != nil {
		err = core.SDKErrorf(err, "", "struct-validation-error", common.GetComponentInfo())
		return
	}

	pathParamsMap := map[string]string{
		"id": *rotateKeyOptions.ID,
	}

	builder := core.NewRequestBuilder(core.POST)
	builder = builder.WithContext(ctx)
	builder.EnableGzipCompression = ibmKeyProtectApi.GetEnableGzipCompression()
	_, err = builder.ResolveRequestURL(ibmKeyProtectApi.Service.Options.URL, `/api/v2/keys/{id}/actions/rotate`, pathParamsMap)
	if err != nil {
		err = core.SDKErrorf(err, "", "url-resolve-error", common.GetComponentInfo())
		return
	}

	sdkHeaders := common.GetSdkHeaders("ibm_key_protect_api", "V2", "RotateKey")
	for headerName, headerValue := range sdkHeaders {
		builder.AddHeader(headerName, headerValue)
	}

	for headerName, headerValue := range rotateKeyOptions.Headers {
		builder.AddHeader(headerName, headerValue)
	}
	builder.AddHeader("Content-Type", "application/vnd.ibm.kms.key_action_rotate+json")
	if rotateKeyOptions.BluemixInstance != nil {
		builder.AddHeader("Bluemix-Instance", fmt.Sprint(*rotateKeyOptions.BluemixInstance))
	}
	if rotateKeyOptions.CorrelationID != nil {
		builder.AddHeader("Correlation-Id", fmt.Sprint(*rotateKeyOptions.CorrelationID))
	}
	if rotateKeyOptions.XKmsKeyRing != nil {
		builder.AddHeader("X-Kms-Key-Ring", fmt.Sprint(*rotateKeyOptions.XKmsKeyRing))
	}
	if rotateKeyOptions.Prefer != nil {
		builder.AddHeader("Prefer", fmt.Sprint(*rotateKeyOptions.Prefer))
	}

	_, err = builder.SetBodyContent("application/vnd.ibm.kms.key_action_rotate+json", nil, nil, rotateKeyOptions.KeyActionRotateBody)
	if err != nil {
		err = core.SDKErrorf(err, "", "set-body-error", common.GetComponentInfo())
		return
	}

	request, err := builder.Build()
	if err != nil {
		err = core.SDKErrorf(err, "", "build-error", common.GetComponentInfo())
		return
	}

	response, err = ibmKeyProtectApi.Service.Request(request, nil)
	if err != nil {
		core.EnrichHTTPProblem(err, "rotateKey", getServiceComponentInfo())
		err = core.SDKErrorf(err, "", "http-request-err", common.GetComponentInfo())
		return
	}

	return
}

// SetKeyForDeletion : Set a key for deletion
// [Authorize deletion](/docs/key-protect?topic=key-protect-delete-dual-auth-keys#set-key-deletion-api) for a key with a
// dual authorization policy.
func (ibmKeyProtectApi *IbmKeyProtectApiV2) SetKeyForDeletion(setKeyForDeletionOptions *SetKeyForDeletionOptions) (response *core.DetailedResponse, err error) {
	response, err = ibmKeyProtectApi.SetKeyForDeletionWithContext(context.Background(), setKeyForDeletionOptions)
	err = core.RepurposeSDKProblem(err, "")
	return
}

// SetKeyForDeletionWithContext is an alternate form of the SetKeyForDeletion method which supports a Context parameter
func (ibmKeyProtectApi *IbmKeyProtectApiV2) SetKeyForDeletionWithContext(ctx context.Context, setKeyForDeletionOptions *SetKeyForDeletionOptions) (response *core.DetailedResponse, err error) {
	err = core.ValidateNotNil(setKeyForDeletionOptions, "setKeyForDeletionOptions cannot be nil")
	if err != nil {
		err = core.SDKErrorf(err, "", "unexpected-nil-param", common.GetComponentInfo())
		return
	}
	err = core.ValidateStruct(setKeyForDeletionOptions, "setKeyForDeletionOptions")
	if err != nil {
		err = core.SDKErrorf(err, "", "struct-validation-error", common.GetComponentInfo())
		return
	}

	pathParamsMap := map[string]string{
		"id": *setKeyForDeletionOptions.ID,
	}

	builder := core.NewRequestBuilder(core.POST)
	builder = builder.WithContext(ctx)
	builder.EnableGzipCompression = ibmKeyProtectApi.GetEnableGzipCompression()
	_, err = builder.ResolveRequestURL(ibmKeyProtectApi.Service.Options.URL, `/api/v2/keys/{id}/actions/setKeyForDeletion`, pathParamsMap)
	if err != nil {
		err = core.SDKErrorf(err, "", "url-resolve-error", common.GetComponentInfo())
		return
	}

	sdkHeaders := common.GetSdkHeaders("ibm_key_protect_api", "V2", "SetKeyForDeletion")
	for headerName, headerValue := range sdkHeaders {
		builder.AddHeader(headerName, headerValue)
	}

	for headerName, headerValue := range setKeyForDeletionOptions.Headers {
		builder.AddHeader(headerName, headerValue)
	}
	if setKeyForDeletionOptions.BluemixInstance != nil {
		builder.AddHeader("Bluemix-Instance", fmt.Sprint(*setKeyForDeletionOptions.BluemixInstance))
	}
	if setKeyForDeletionOptions.CorrelationID != nil {
		builder.AddHeader("Correlation-Id", fmt.Sprint(*setKeyForDeletionOptions.CorrelationID))
	}
	if setKeyForDeletionOptions.XKmsKeyRing != nil {
		builder.AddHeader("X-Kms-Key-Ring", fmt.Sprint(*setKeyForDeletionOptions.XKmsKeyRing))
	}

	request, err := builder.Build()
	if err != nil {
		err = core.SDKErrorf(err, "", "build-error", common.GetComponentInfo())
		return
	}

	response, err = ibmKeyProtectApi.Service.Request(request, nil)
	if err != nil {
		core.EnrichHTTPProblem(err, "setKeyForDeletion", getServiceComponentInfo())
		err = core.SDKErrorf(err, "", "http-request-err", common.GetComponentInfo())
		return
	}

	return
}

// UnsetKeyForDeletion : Unset a key for deletion
// [Remove an authorization](/docs/key-protect?topic=key-protect-delete-dual-auth-keys#unset-key-deletion-api) for a key
// with a dual authorization policy.
func (ibmKeyProtectApi *IbmKeyProtectApiV2) UnsetKeyForDeletion(unsetKeyForDeletionOptions *UnsetKeyForDeletionOptions) (response *core.DetailedResponse, err error) {
	response, err = ibmKeyProtectApi.UnsetKeyForDeletionWithContext(context.Background(), unsetKeyForDeletionOptions)
	err = core.RepurposeSDKProblem(err, "")
	return
}

// UnsetKeyForDeletionWithContext is an alternate form of the UnsetKeyForDeletion method which supports a Context parameter
func (ibmKeyProtectApi *IbmKeyProtectApiV2) UnsetKeyForDeletionWithContext(ctx context.Context, unsetKeyForDeletionOptions *UnsetKeyForDeletionOptions) (response *core.DetailedResponse, err error) {
	err = core.ValidateNotNil(unsetKeyForDeletionOptions, "unsetKeyForDeletionOptions cannot be nil")
	if err != nil {
		err = core.SDKErrorf(err, "", "unexpected-nil-param", common.GetComponentInfo())
		return
	}
	err = core.ValidateStruct(unsetKeyForDeletionOptions, "unsetKeyForDeletionOptions")
	if err != nil {
		err = core.SDKErrorf(err, "", "struct-validation-error", common.GetComponentInfo())
		return
	}

	pathParamsMap := map[string]string{
		"id": *unsetKeyForDeletionOptions.ID,
	}

	builder := core.NewRequestBuilder(core.POST)
	builder = builder.WithContext(ctx)
	builder.EnableGzipCompression = ibmKeyProtectApi.GetEnableGzipCompression()
	_, err = builder.ResolveRequestURL(ibmKeyProtectApi.Service.Options.URL, `/api/v2/keys/{id}/actions/unsetKeyForDeletion`, pathParamsMap)
	if err != nil {
		err = core.SDKErrorf(err, "", "url-resolve-error", common.GetComponentInfo())
		return
	}

	sdkHeaders := common.GetSdkHeaders("ibm_key_protect_api", "V2", "UnsetKeyForDeletion")
	for headerName, headerValue := range sdkHeaders {
		builder.AddHeader(headerName, headerValue)
	}

	for headerName, headerValue := range unsetKeyForDeletionOptions.Headers {
		builder.AddHeader(headerName, headerValue)
	}
	if unsetKeyForDeletionOptions.BluemixInstance != nil {
		builder.AddHeader("Bluemix-Instance", fmt.Sprint(*unsetKeyForDeletionOptions.BluemixInstance))
	}
	if unsetKeyForDeletionOptions.CorrelationID != nil {
		builder.AddHeader("Correlation-Id", fmt.Sprint(*unsetKeyForDeletionOptions.CorrelationID))
	}
	if unsetKeyForDeletionOptions.XKmsKeyRing != nil {
		builder.AddHeader("X-Kms-Key-Ring", fmt.Sprint(*unsetKeyForDeletionOptions.XKmsKeyRing))
	}

	request, err := builder.Build()
	if err != nil {
		err = core.SDKErrorf(err, "", "build-error", common.GetComponentInfo())
		return
	}

	response, err = ibmKeyProtectApi.Service.Request(request, nil)
	if err != nil {
		core.EnrichHTTPProblem(err, "unsetKeyForDeletion", getServiceComponentInfo())
		err = core.SDKErrorf(err, "", "http-request-err", common.GetComponentInfo())
		return
	}

	return
}

// EnableKey : Enable a key
// [Enable operations](/docs/key-protect?topic=key-protect-disable-keys#enable-api) for a key.
func (ibmKeyProtectApi *IbmKeyProtectApiV2) EnableKey(enableKeyOptions *EnableKeyOptions) (response *core.DetailedResponse, err error) {
	response, err = ibmKeyProtectApi.EnableKeyWithContext(context.Background(), enableKeyOptions)
	err = core.RepurposeSDKProblem(err, "")
	return
}

// EnableKeyWithContext is an alternate form of the EnableKey method which supports a Context parameter
func (ibmKeyProtectApi *IbmKeyProtectApiV2) EnableKeyWithContext(ctx context.Context, enableKeyOptions *EnableKeyOptions) (response *core.DetailedResponse, err error) {
	err = core.ValidateNotNil(enableKeyOptions, "enableKeyOptions cannot be nil")
	if err != nil {
		err = core.SDKErrorf(err, "", "unexpected-nil-param", common.GetComponentInfo())
		return
	}
	err = core.ValidateStruct(enableKeyOptions, "enableKeyOptions")
	if err != nil {
		err = core.SDKErrorf(err, "", "struct-validation-error", common.GetComponentInfo())
		return
	}

	pathParamsMap := map[string]string{
		"id": *enableKeyOptions.ID,
	}

	builder := core.NewRequestBuilder(core.POST)
	builder = builder.WithContext(ctx)
	builder.EnableGzipCompression = ibmKeyProtectApi.GetEnableGzipCompression()
	_, err = builder.ResolveRequestURL(ibmKeyProtectApi.Service.Options.URL, `/api/v2/keys/{id}/actions/enable`, pathParamsMap)
	if err != nil {
		err = core.SDKErrorf(err, "", "url-resolve-error", common.GetComponentInfo())
		return
	}

	sdkHeaders := common.GetSdkHeaders("ibm_key_protect_api", "V2", "EnableKey")
	for headerName, headerValue := range sdkHeaders {
		builder.AddHeader(headerName, headerValue)
	}

	for headerName, headerValue := range enableKeyOptions.Headers {
		builder.AddHeader(headerName, headerValue)
	}
	if enableKeyOptions.BluemixInstance != nil {
		builder.AddHeader("Bluemix-Instance", fmt.Sprint(*enableKeyOptions.BluemixInstance))
	}
	if enableKeyOptions.CorrelationID != nil {
		builder.AddHeader("Correlation-Id", fmt.Sprint(*enableKeyOptions.CorrelationID))
	}
	if enableKeyOptions.XKmsKeyRing != nil {
		builder.AddHeader("X-Kms-Key-Ring", fmt.Sprint(*enableKeyOptions.XKmsKeyRing))
	}

	request, err := builder.Build()
	if err != nil {
		err = core.SDKErrorf(err, "", "build-error", common.GetComponentInfo())
		return
	}

	response, err = ibmKeyProtectApi.Service.Request(request, nil)
	if err != nil {
		core.EnrichHTTPProblem(err, "enableKey", getServiceComponentInfo())
		err = core.SDKErrorf(err, "", "http-request-err", common.GetComponentInfo())
		return
	}

	return
}

// DisableKey : Disable a key
// [Disable operations](/docs/key-protect?topic=key-protect-disable-keys) for a key.
func (ibmKeyProtectApi *IbmKeyProtectApiV2) DisableKey(disableKeyOptions *DisableKeyOptions) (response *core.DetailedResponse, err error) {
	response, err = ibmKeyProtectApi.DisableKeyWithContext(context.Background(), disableKeyOptions)
	err = core.RepurposeSDKProblem(err, "")
	return
}

// DisableKeyWithContext is an alternate form of the DisableKey method which supports a Context parameter
func (ibmKeyProtectApi *IbmKeyProtectApiV2) DisableKeyWithContext(ctx context.Context, disableKeyOptions *DisableKeyOptions) (response *core.DetailedResponse, err error) {
	err = core.ValidateNotNil(disableKeyOptions, "disableKeyOptions cannot be nil")
	if err != nil {
		err = core.SDKErrorf(err, "", "unexpected-nil-param", common.GetComponentInfo())
		return
	}
	err = core.ValidateStruct(disableKeyOptions, "disableKeyOptions")
	if err != nil {
		err = core.SDKErrorf(err, "", "struct-validation-error", common.GetComponentInfo())
		return
	}

	pathParamsMap := map[string]string{
		"id": *disableKeyOptions.ID,
	}

	builder := core.NewRequestBuilder(core.POST)
	builder = builder.WithContext(ctx)
	builder.EnableGzipCompression = ibmKeyProtectApi.GetEnableGzipCompression()
	_, err = builder.ResolveRequestURL(ibmKeyProtectApi.Service.Options.URL, `/api/v2/keys/{id}/actions/disable`, pathParamsMap)
	if err != nil {
		err = core.SDKErrorf(err, "", "url-resolve-error", common.GetComponentInfo())
		return
	}

	sdkHeaders := common.GetSdkHeaders("ibm_key_protect_api", "V2", "DisableKey")
	for headerName, headerValue := range sdkHeaders {
		builder.AddHeader(headerName, headerValue)
	}

	for headerName, headerValue := range disableKeyOptions.Headers {
		builder.AddHeader(headerName, headerValue)
	}
	if disableKeyOptions.BluemixInstance != nil {
		builder.AddHeader("Bluemix-Instance", fmt.Sprint(*disableKeyOptions.BluemixInstance))
	}
	if disableKeyOptions.CorrelationID != nil {
		builder.AddHeader("Correlation-Id", fmt.Sprint(*disableKeyOptions.CorrelationID))
	}
	if disableKeyOptions.XKmsKeyRing != nil {
		builder.AddHeader("X-Kms-Key-Ring", fmt.Sprint(*disableKeyOptions.XKmsKeyRing))
	}

	request, err := builder.Build()
	if err != nil {
		err = core.SDKErrorf(err, "", "build-error", common.GetComponentInfo())
		return
	}

	response, err = ibmKeyProtectApi.Service.Request(request, nil)
	if err != nil {
		core.EnrichHTTPProblem(err, "disableKey", getServiceComponentInfo())
		err = core.SDKErrorf(err, "", "http-request-err", common.GetComponentInfo())
		return
	}

	return
}

// SyncAssociatedResources : Sync associated resources
// Initiate a [manual data synchronization](/docs/key-protect?topic=key-protect-sync-associated-resources&interface=api)
// request to the associated resources of a key. Regular key lifecycle events automatically notify integrated services
// of any change. However, in the case a service does not respond to a key lifecycle event notification after four
// hours, the
// `sync` API may be used to initiate a renotification to the integrated services that manage the associated resources
// linked to the key.
//
// **Note:** The services that manage the associated resources linked to the key are responsible for maintaining current
// records of the key state and version. Key Protect does not have the ability to force data synchronization for other
// services, which may take up to four hours to complete. The `sync` API is meant to **initiate** a request for all
// associated resources to synchronize their key records with the information returned from the Key Protect API.
func (ibmKeyProtectApi *IbmKeyProtectApiV2) SyncAssociatedResources(syncAssociatedResourcesOptions *SyncAssociatedResourcesOptions) (response *core.DetailedResponse, err error) {
	response, err = ibmKeyProtectApi.SyncAssociatedResourcesWithContext(context.Background(), syncAssociatedResourcesOptions)
	err = core.RepurposeSDKProblem(err, "")
	return
}

// SyncAssociatedResourcesWithContext is an alternate form of the SyncAssociatedResources method which supports a Context parameter
func (ibmKeyProtectApi *IbmKeyProtectApiV2) SyncAssociatedResourcesWithContext(ctx context.Context, syncAssociatedResourcesOptions *SyncAssociatedResourcesOptions) (response *core.DetailedResponse, err error) {
	err = core.ValidateNotNil(syncAssociatedResourcesOptions, "syncAssociatedResourcesOptions cannot be nil")
	if err != nil {
		err = core.SDKErrorf(err, "", "unexpected-nil-param", common.GetComponentInfo())
		return
	}
	err = core.ValidateStruct(syncAssociatedResourcesOptions, "syncAssociatedResourcesOptions")
	if err != nil {
		err = core.SDKErrorf(err, "", "struct-validation-error", common.GetComponentInfo())
		return
	}

	pathParamsMap := map[string]string{
		"id": *syncAssociatedResourcesOptions.ID,
	}

	builder := core.NewRequestBuilder(core.POST)
	builder = builder.WithContext(ctx)
	builder.EnableGzipCompression = ibmKeyProtectApi.GetEnableGzipCompression()
	_, err = builder.ResolveRequestURL(ibmKeyProtectApi.Service.Options.URL, `/api/v2/keys/{id}/actions/sync`, pathParamsMap)
	if err != nil {
		err = core.SDKErrorf(err, "", "url-resolve-error", common.GetComponentInfo())
		return
	}

	sdkHeaders := common.GetSdkHeaders("ibm_key_protect_api", "V2", "SyncAssociatedResources")
	for headerName, headerValue := range sdkHeaders {
		builder.AddHeader(headerName, headerValue)
	}

	for headerName, headerValue := range syncAssociatedResourcesOptions.Headers {
		builder.AddHeader(headerName, headerValue)
	}
	if syncAssociatedResourcesOptions.BluemixInstance != nil {
		builder.AddHeader("Bluemix-Instance", fmt.Sprint(*syncAssociatedResourcesOptions.BluemixInstance))
	}
	if syncAssociatedResourcesOptions.CorrelationID != nil {
		builder.AddHeader("Correlation-Id", fmt.Sprint(*syncAssociatedResourcesOptions.CorrelationID))
	}
	if syncAssociatedResourcesOptions.XKmsKeyRing != nil {
		builder.AddHeader("X-Kms-Key-Ring", fmt.Sprint(*syncAssociatedResourcesOptions.XKmsKeyRing))
	}

	request, err := builder.Build()
	if err != nil {
		err = core.SDKErrorf(err, "", "build-error", common.GetComponentInfo())
		return
	}

	response, err = ibmKeyProtectApi.Service.Request(request, nil)
	if err != nil {
		core.EnrichHTTPProblem(err, "syncAssociatedResources", getServiceComponentInfo())
		err = core.SDKErrorf(err, "", "http-request-err", common.GetComponentInfo())
		return
	}

	return
}

// PutPolicy : Set key policies
// Creates or updates one or more policies for the specified key.
//
// You can set policies for a key, such as an
// [automatic rotation policy](/docs/key-protect?topic=key-protect-set-rotation-policy) or a
// [dual authorization policy](/docs/key-protect?topic=key-protect-set-dual-auth-key-policy) to protect against the
// accidental deletion of keys. Use
// `PUT /keys/{id}/policies` to create new policies for a key or update an existing policy.
func (ibmKeyProtectApi *IbmKeyProtectApiV2) PutPolicy(putPolicyOptions *PutPolicyOptions) (result GetKeyPoliciesOneOfIntf, response *core.DetailedResponse, err error) {
	result, response, err = ibmKeyProtectApi.PutPolicyWithContext(context.Background(), putPolicyOptions)
	err = core.RepurposeSDKProblem(err, "")
	return
}

// PutPolicyWithContext is an alternate form of the PutPolicy method which supports a Context parameter
func (ibmKeyProtectApi *IbmKeyProtectApiV2) PutPolicyWithContext(ctx context.Context, putPolicyOptions *PutPolicyOptions) (result GetKeyPoliciesOneOfIntf, response *core.DetailedResponse, err error) {
	err = core.ValidateNotNil(putPolicyOptions, "putPolicyOptions cannot be nil")
	if err != nil {
		err = core.SDKErrorf(err, "", "unexpected-nil-param", common.GetComponentInfo())
		return
	}
	err = core.ValidateStruct(putPolicyOptions, "putPolicyOptions")
	if err != nil {
		err = core.SDKErrorf(err, "", "struct-validation-error", common.GetComponentInfo())
		return
	}

	pathParamsMap := map[string]string{
		"id": *putPolicyOptions.ID,
	}

	builder := core.NewRequestBuilder(core.PUT)
	builder = builder.WithContext(ctx)
	builder.EnableGzipCompression = ibmKeyProtectApi.GetEnableGzipCompression()
	_, err = builder.ResolveRequestURL(ibmKeyProtectApi.Service.Options.URL, `/api/v2/keys/{id}/policies`, pathParamsMap)
	if err != nil {
		err = core.SDKErrorf(err, "", "url-resolve-error", common.GetComponentInfo())
		return
	}

	sdkHeaders := common.GetSdkHeaders("ibm_key_protect_api", "V2", "PutPolicy")
	for headerName, headerValue := range sdkHeaders {
		builder.AddHeader(headerName, headerValue)
	}

	for headerName, headerValue := range putPolicyOptions.Headers {
		builder.AddHeader(headerName, headerValue)
	}
	builder.AddHeader("Accept", "application/json")
	builder.AddHeader("Content-Type", "application/json")
	if putPolicyOptions.BluemixInstance != nil {
		builder.AddHeader("Bluemix-Instance", fmt.Sprint(*putPolicyOptions.BluemixInstance))
	}
	if putPolicyOptions.CorrelationID != nil {
		builder.AddHeader("Correlation-Id", fmt.Sprint(*putPolicyOptions.CorrelationID))
	}
	if putPolicyOptions.XKmsKeyRing != nil {
		builder.AddHeader("X-Kms-Key-Ring", fmt.Sprint(*putPolicyOptions.XKmsKeyRing))
	}

	if putPolicyOptions.Policy != nil {
		builder.AddQuery("policy", fmt.Sprint(*putPolicyOptions.Policy))
	}

	_, err = builder.SetBodyContentJSON(putPolicyOptions.KeyPolicyPutBody)
	if err != nil {
		err = core.SDKErrorf(err, "", "set-json-body-error", common.GetComponentInfo())
		return
	}

	request, err := builder.Build()
	if err != nil {
		err = core.SDKErrorf(err, "", "build-error", common.GetComponentInfo())
		return
	}

	var rawResponse map[string]json.RawMessage
	response, err = ibmKeyProtectApi.Service.Request(request, &rawResponse)
	if err != nil {
		core.EnrichHTTPProblem(err, "putPolicy", getServiceComponentInfo())
		err = core.SDKErrorf(err, "", "http-request-err", common.GetComponentInfo())
		return
	}
	if rawResponse != nil {
		err = core.UnmarshalModel(rawResponse, "", &result, UnmarshalGetKeyPoliciesOneOf)
		if err != nil {
			err = core.SDKErrorf(err, "", "unmarshal-resp-error", common.GetComponentInfo())
			return
		}
		response.Result = result
	}

	return
}

// GetPolicy : List key policies
// Retrieves a list of policies that are associated with a specified key.
//
// You can set policies for a key, such as an
// [automatic rotation policy](/docs/key-protect?topic=key-protect-set-rotation-policy) or a
// [dual authorization policy](/docs/key-protect?topic=key-protect-set-dual-auth-key-policy) to protect against the
// accidental deletion of keys. Use
// `GET /keys/{id}/policies` to browse the policies that exist for a specified key.
func (ibmKeyProtectApi *IbmKeyProtectApiV2) GetPolicy(getPolicyOptions *GetPolicyOptions) (result GetKeyPoliciesOneOfIntf, response *core.DetailedResponse, err error) {
	result, response, err = ibmKeyProtectApi.GetPolicyWithContext(context.Background(), getPolicyOptions)
	err = core.RepurposeSDKProblem(err, "")
	return
}

// GetPolicyWithContext is an alternate form of the GetPolicy method which supports a Context parameter
func (ibmKeyProtectApi *IbmKeyProtectApiV2) GetPolicyWithContext(ctx context.Context, getPolicyOptions *GetPolicyOptions) (result GetKeyPoliciesOneOfIntf, response *core.DetailedResponse, err error) {
	err = core.ValidateNotNil(getPolicyOptions, "getPolicyOptions cannot be nil")
	if err != nil {
		err = core.SDKErrorf(err, "", "unexpected-nil-param", common.GetComponentInfo())
		return
	}
	err = core.ValidateStruct(getPolicyOptions, "getPolicyOptions")
	if err != nil {
		err = core.SDKErrorf(err, "", "struct-validation-error", common.GetComponentInfo())
		return
	}

	pathParamsMap := map[string]string{
		"id": *getPolicyOptions.ID,
	}

	builder := core.NewRequestBuilder(core.GET)
	builder = builder.WithContext(ctx)
	builder.EnableGzipCompression = ibmKeyProtectApi.GetEnableGzipCompression()
	_, err = builder.ResolveRequestURL(ibmKeyProtectApi.Service.Options.URL, `/api/v2/keys/{id}/policies`, pathParamsMap)
	if err != nil {
		err = core.SDKErrorf(err, "", "url-resolve-error", common.GetComponentInfo())
		return
	}

	sdkHeaders := common.GetSdkHeaders("ibm_key_protect_api", "V2", "GetPolicy")
	for headerName, headerValue := range sdkHeaders {
		builder.AddHeader(headerName, headerValue)
	}

	for headerName, headerValue := range getPolicyOptions.Headers {
		builder.AddHeader(headerName, headerValue)
	}
	builder.AddHeader("Accept", "application/json")
	if getPolicyOptions.BluemixInstance != nil {
		builder.AddHeader("Bluemix-Instance", fmt.Sprint(*getPolicyOptions.BluemixInstance))
	}
	if getPolicyOptions.CorrelationID != nil {
		builder.AddHeader("Correlation-Id", fmt.Sprint(*getPolicyOptions.CorrelationID))
	}
	if getPolicyOptions.XKmsKeyRing != nil {
		builder.AddHeader("X-Kms-Key-Ring", fmt.Sprint(*getPolicyOptions.XKmsKeyRing))
	}

	if getPolicyOptions.Policy != nil {
		builder.AddQuery("policy", fmt.Sprint(*getPolicyOptions.Policy))
	}

	request, err := builder.Build()
	if err != nil {
		err = core.SDKErrorf(err, "", "build-error", common.GetComponentInfo())
		return
	}

	var rawResponse map[string]json.RawMessage
	response, err = ibmKeyProtectApi.Service.Request(request, &rawResponse)
	if err != nil {
		core.EnrichHTTPProblem(err, "getPolicy", getServiceComponentInfo())
		err = core.SDKErrorf(err, "", "http-request-err", common.GetComponentInfo())
		return
	}
	if rawResponse != nil {
		err = core.UnmarshalModel(rawResponse, "", &result, UnmarshalGetKeyPoliciesOneOf)
		if err != nil {
			err = core.SDKErrorf(err, "", "unmarshal-resp-error", common.GetComponentInfo())
			return
		}
		response.Result = result
	}

	return
}

// PutInstancePolicy : Set instance policies
// Creates or updates one or more policies for the specified service instance.
//
// **Note:** When you set an instance policy, Key Protect associates the policy information with keys that you add to
// the instance after the policy is updated. This operation does not affect existing keys in the instance.
func (ibmKeyProtectApi *IbmKeyProtectApiV2) PutInstancePolicy(putInstancePolicyOptions *PutInstancePolicyOptions) (response *core.DetailedResponse, err error) {
	response, err = ibmKeyProtectApi.PutInstancePolicyWithContext(context.Background(), putInstancePolicyOptions)
	err = core.RepurposeSDKProblem(err, "")
	return
}

// PutInstancePolicyWithContext is an alternate form of the PutInstancePolicy method which supports a Context parameter
func (ibmKeyProtectApi *IbmKeyProtectApiV2) PutInstancePolicyWithContext(ctx context.Context, putInstancePolicyOptions *PutInstancePolicyOptions) (response *core.DetailedResponse, err error) {
	err = core.ValidateNotNil(putInstancePolicyOptions, "putInstancePolicyOptions cannot be nil")
	if err != nil {
		err = core.SDKErrorf(err, "", "unexpected-nil-param", common.GetComponentInfo())
		return
	}
	err = core.ValidateStruct(putInstancePolicyOptions, "putInstancePolicyOptions")
	if err != nil {
		err = core.SDKErrorf(err, "", "struct-validation-error", common.GetComponentInfo())
		return
	}

	builder := core.NewRequestBuilder(core.PUT)
	builder = builder.WithContext(ctx)
	builder.EnableGzipCompression = ibmKeyProtectApi.GetEnableGzipCompression()
	_, err = builder.ResolveRequestURL(ibmKeyProtectApi.Service.Options.URL, `/api/v2/instance/policies`, nil)
	if err != nil {
		err = core.SDKErrorf(err, "", "url-resolve-error", common.GetComponentInfo())
		return
	}

	sdkHeaders := common.GetSdkHeaders("ibm_key_protect_api", "V2", "PutInstancePolicy")
	for headerName, headerValue := range sdkHeaders {
		builder.AddHeader(headerName, headerValue)
	}

	for headerName, headerValue := range putInstancePolicyOptions.Headers {
		builder.AddHeader(headerName, headerValue)
	}
	builder.AddHeader("Content-Type", "application/json")
	if putInstancePolicyOptions.BluemixInstance != nil {
		builder.AddHeader("Bluemix-Instance", fmt.Sprint(*putInstancePolicyOptions.BluemixInstance))
	}
	if putInstancePolicyOptions.CorrelationID != nil {
		builder.AddHeader("Correlation-Id", fmt.Sprint(*putInstancePolicyOptions.CorrelationID))
	}

	if putInstancePolicyOptions.Policy != nil {
		builder.AddQuery("policy", fmt.Sprint(*putInstancePolicyOptions.Policy))
	}

	_, err = builder.SetBodyContentJSON(putInstancePolicyOptions.InstancePolicyPutBody)
	if err != nil {
		err = core.SDKErrorf(err, "", "set-json-body-error", common.GetComponentInfo())
		return
	}

	request, err := builder.Build()
	if err != nil {
		err = core.SDKErrorf(err, "", "build-error", common.GetComponentInfo())
		return
	}

	response, err = ibmKeyProtectApi.Service.Request(request, nil)
	if err != nil {
		core.EnrichHTTPProblem(err, "putInstancePolicy", getServiceComponentInfo())
		err = core.SDKErrorf(err, "", "http-request-err", common.GetComponentInfo())
		return
	}

	return
}

// GetInstancePolicy : List instance policies
// Retrieves a list of policies that are associated with a specified service instance.
//
// You can manage advanced preferences for keys in your service instance by creating instance-level policies. Use `GET
// /instance/policies` to browse the policies that are associated with the specified instance. Currently, dual
// authorization policies are supported.
func (ibmKeyProtectApi *IbmKeyProtectApiV2) GetInstancePolicy(getInstancePolicyOptions *GetInstancePolicyOptions) (result GetInstancePoliciesOneOfIntf, response *core.DetailedResponse, err error) {
	result, response, err = ibmKeyProtectApi.GetInstancePolicyWithContext(context.Background(), getInstancePolicyOptions)
	err = core.RepurposeSDKProblem(err, "")
	return
}

// GetInstancePolicyWithContext is an alternate form of the GetInstancePolicy method which supports a Context parameter
func (ibmKeyProtectApi *IbmKeyProtectApiV2) GetInstancePolicyWithContext(ctx context.Context, getInstancePolicyOptions *GetInstancePolicyOptions) (result GetInstancePoliciesOneOfIntf, response *core.DetailedResponse, err error) {
	err = core.ValidateNotNil(getInstancePolicyOptions, "getInstancePolicyOptions cannot be nil")
	if err != nil {
		err = core.SDKErrorf(err, "", "unexpected-nil-param", common.GetComponentInfo())
		return
	}
	err = core.ValidateStruct(getInstancePolicyOptions, "getInstancePolicyOptions")
	if err != nil {
		err = core.SDKErrorf(err, "", "struct-validation-error", common.GetComponentInfo())
		return
	}

	builder := core.NewRequestBuilder(core.GET)
	builder = builder.WithContext(ctx)
	builder.EnableGzipCompression = ibmKeyProtectApi.GetEnableGzipCompression()
	_, err = builder.ResolveRequestURL(ibmKeyProtectApi.Service.Options.URL, `/api/v2/instance/policies`, nil)
	if err != nil {
		err = core.SDKErrorf(err, "", "url-resolve-error", common.GetComponentInfo())
		return
	}

	sdkHeaders := common.GetSdkHeaders("ibm_key_protect_api", "V2", "GetInstancePolicy")
	for headerName, headerValue := range sdkHeaders {
		builder.AddHeader(headerName, headerValue)
	}

	for headerName, headerValue := range getInstancePolicyOptions.Headers {
		builder.AddHeader(headerName, headerValue)
	}
	builder.AddHeader("Accept", "application/json")
	if getInstancePolicyOptions.BluemixInstance != nil {
		builder.AddHeader("Bluemix-Instance", fmt.Sprint(*getInstancePolicyOptions.BluemixInstance))
	}
	if getInstancePolicyOptions.CorrelationID != nil {
		builder.AddHeader("Correlation-Id", fmt.Sprint(*getInstancePolicyOptions.CorrelationID))
	}

	if getInstancePolicyOptions.Policy != nil {
		builder.AddQuery("policy", fmt.Sprint(*getInstancePolicyOptions.Policy))
	}

	request, err := builder.Build()
	if err != nil {
		err = core.SDKErrorf(err, "", "build-error", common.GetComponentInfo())
		return
	}

	var rawResponse map[string]json.RawMessage
	response, err = ibmKeyProtectApi.Service.Request(request, &rawResponse)
	if err != nil {
		core.EnrichHTTPProblem(err, "getInstancePolicy", getServiceComponentInfo())
		err = core.SDKErrorf(err, "", "http-request-err", common.GetComponentInfo())
		return
	}
	if rawResponse != nil {
		err = core.UnmarshalModel(rawResponse, "", &result, UnmarshalGetInstancePoliciesOneOf)
		if err != nil {
			err = core.SDKErrorf(err, "", "unmarshal-resp-error", common.GetComponentInfo())
			return
		}
		response.Result = result
	}

	return
}

// GetAllowedIPPort : Retrieve allowed IP port
// Retrieves the private endpoint port associated with your service instance's active allowed IP policy. If the instance
// does not contain an active allowed IP policy, no information will be returned.
func (ibmKeyProtectApi *IbmKeyProtectApiV2) GetAllowedIPPort(getAllowedIPPortOptions *GetAllowedIPPortOptions) (result *AllowedIPPort, response *core.DetailedResponse, err error) {
	result, response, err = ibmKeyProtectApi.GetAllowedIPPortWithContext(context.Background(), getAllowedIPPortOptions)
	err = core.RepurposeSDKProblem(err, "")
	return
}

// GetAllowedIPPortWithContext is an alternate form of the GetAllowedIPPort method which supports a Context parameter
func (ibmKeyProtectApi *IbmKeyProtectApiV2) GetAllowedIPPortWithContext(ctx context.Context, getAllowedIPPortOptions *GetAllowedIPPortOptions) (result *AllowedIPPort, response *core.DetailedResponse, err error) {
	err = core.ValidateNotNil(getAllowedIPPortOptions, "getAllowedIPPortOptions cannot be nil")
	if err != nil {
		err = core.SDKErrorf(err, "", "unexpected-nil-param", common.GetComponentInfo())
		return
	}
	err = core.ValidateStruct(getAllowedIPPortOptions, "getAllowedIPPortOptions")
	if err != nil {
		err = core.SDKErrorf(err, "", "struct-validation-error", common.GetComponentInfo())
		return
	}

	builder := core.NewRequestBuilder(core.GET)
	builder = builder.WithContext(ctx)
	builder.EnableGzipCompression = ibmKeyProtectApi.GetEnableGzipCompression()
	_, err = builder.ResolveRequestURL(ibmKeyProtectApi.Service.Options.URL, `/api/v2/instance/allowed_ip_port`, nil)
	if err != nil {
		err = core.SDKErrorf(err, "", "url-resolve-error", common.GetComponentInfo())
		return
	}

	sdkHeaders := common.GetSdkHeaders("ibm_key_protect_api", "V2", "GetAllowedIPPort")
	for headerName, headerValue := range sdkHeaders {
		builder.AddHeader(headerName, headerValue)
	}

	for headerName, headerValue := range getAllowedIPPortOptions.Headers {
		builder.AddHeader(headerName, headerValue)
	}
	builder.AddHeader("Accept", "application/json")
	if getAllowedIPPortOptions.BluemixInstance != nil {
		builder.AddHeader("Bluemix-Instance", fmt.Sprint(*getAllowedIPPortOptions.BluemixInstance))
	}
	if getAllowedIPPortOptions.CorrelationID != nil {
		builder.AddHeader("Correlation-Id", fmt.Sprint(*getAllowedIPPortOptions.CorrelationID))
	}

	request, err := builder.Build()
	if err != nil {
		err = core.SDKErrorf(err, "", "build-error", common.GetComponentInfo())
		return
	}

	var rawResponse map[string]json.RawMessage
	response, err = ibmKeyProtectApi.Service.Request(request, &rawResponse)
	if err != nil {
		core.EnrichHTTPProblem(err, "getAllowedIPPort", getServiceComponentInfo())
		err = core.SDKErrorf(err, "", "http-request-err", common.GetComponentInfo())
		return
	}
	if rawResponse != nil {
		err = core.UnmarshalModel(rawResponse, "", &result, UnmarshalAllowedIPPort)
		if err != nil {
			err = core.SDKErrorf(err, "", "unmarshal-resp-error", common.GetComponentInfo())
			return
		}
		response.Result = result
	}

	return
}

// PostImportToken : Create an import token
// Creates an import token that you can use to encrypt and import root keys into the service.
// [Learn more](/docs/key-protect?topic=key-protect-importing-keys#using-import-tokens).
//
// When you call `POST /import_token`, Key Protect creates an RSA key-pair from its HSMs. The service encrypts and
// stores the private key in the HSM, and returns the corresponding public key when you call
// `GET /import_token`. You can create only one import token per service instance.
func (ibmKeyProtectApi *IbmKeyProtectApiV2) PostImportToken(postImportTokenOptions *PostImportTokenOptions) (result *ImportToken, response *core.DetailedResponse, err error) {
	result, response, err = ibmKeyProtectApi.PostImportTokenWithContext(context.Background(), postImportTokenOptions)
	err = core.RepurposeSDKProblem(err, "")
	return
}

// PostImportTokenWithContext is an alternate form of the PostImportToken method which supports a Context parameter
func (ibmKeyProtectApi *IbmKeyProtectApiV2) PostImportTokenWithContext(ctx context.Context, postImportTokenOptions *PostImportTokenOptions) (result *ImportToken, response *core.DetailedResponse, err error) {
	err = core.ValidateNotNil(postImportTokenOptions, "postImportTokenOptions cannot be nil")
	if err != nil {
		err = core.SDKErrorf(err, "", "unexpected-nil-param", common.GetComponentInfo())
		return
	}
	err = core.ValidateStruct(postImportTokenOptions, "postImportTokenOptions")
	if err != nil {
		err = core.SDKErrorf(err, "", "struct-validation-error", common.GetComponentInfo())
		return
	}

	builder := core.NewRequestBuilder(core.POST)
	builder = builder.WithContext(ctx)
	builder.EnableGzipCompression = ibmKeyProtectApi.GetEnableGzipCompression()
	_, err = builder.ResolveRequestURL(ibmKeyProtectApi.Service.Options.URL, `/api/v2/import_token`, nil)
	if err != nil {
		err = core.SDKErrorf(err, "", "url-resolve-error", common.GetComponentInfo())
		return
	}

	sdkHeaders := common.GetSdkHeaders("ibm_key_protect_api", "V2", "PostImportToken")
	for headerName, headerValue := range sdkHeaders {
		builder.AddHeader(headerName, headerValue)
	}

	for headerName, headerValue := range postImportTokenOptions.Headers {
		builder.AddHeader(headerName, headerValue)
	}
	builder.AddHeader("Accept", "application/json")
	builder.AddHeader("Content-Type", "application/json")
	if postImportTokenOptions.BluemixInstance != nil {
		builder.AddHeader("Bluemix-Instance", fmt.Sprint(*postImportTokenOptions.BluemixInstance))
	}
	if postImportTokenOptions.CorrelationID != nil {
		builder.AddHeader("Correlation-Id", fmt.Sprint(*postImportTokenOptions.CorrelationID))
	}
	if postImportTokenOptions.XKmsKeyRing != nil {
		builder.AddHeader("X-Kms-Key-Ring", fmt.Sprint(*postImportTokenOptions.XKmsKeyRing))
	}

	body := make(map[string]interface{})
	if postImportTokenOptions.Expiration != nil {
		body["expiration"] = postImportTokenOptions.Expiration
	}
	if postImportTokenOptions.MaxAllowedRetrievals != nil {
		body["maxAllowedRetrievals"] = postImportTokenOptions.MaxAllowedRetrievals
	}
	_, err = builder.SetBodyContentJSON(body)
	if err != nil {
		err = core.SDKErrorf(err, "", "set-json-body-error", common.GetComponentInfo())
		return
	}

	request, err := builder.Build()
	if err != nil {
		err = core.SDKErrorf(err, "", "build-error", common.GetComponentInfo())
		return
	}

	var rawResponse map[string]json.RawMessage
	response, err = ibmKeyProtectApi.Service.Request(request, &rawResponse)
	if err != nil {
		core.EnrichHTTPProblem(err, "postImportToken", getServiceComponentInfo())
		err = core.SDKErrorf(err, "", "http-request-err", common.GetComponentInfo())
		return
	}
	if rawResponse != nil {
		err = core.UnmarshalModel(rawResponse, "", &result, UnmarshalImportToken)
		if err != nil {
			err = core.SDKErrorf(err, "", "unmarshal-resp-error", common.GetComponentInfo())
			return
		}
		response.Result = result
	}

	return
}

// GetImportToken : Retrieve an import token
// Retrieves the import token that is associated with your service instance.
//
// When you call `GET /import_token`, Key Protect returns the public key that you can use to encrypt and import key
// material to the service, along with details about the key.
//
// **Note:** After you reach the `maxAllowedRetrievals` or `expirationDate` for the import token, the import token and
// its associated public key can no longer be used for key operations. To create a new import token, use
// `POST /import_token`.
func (ibmKeyProtectApi *IbmKeyProtectApiV2) GetImportToken(getImportTokenOptions *GetImportTokenOptions) (result *GetImportToken, response *core.DetailedResponse, err error) {
	result, response, err = ibmKeyProtectApi.GetImportTokenWithContext(context.Background(), getImportTokenOptions)
	err = core.RepurposeSDKProblem(err, "")
	return
}

// GetImportTokenWithContext is an alternate form of the GetImportToken method which supports a Context parameter
func (ibmKeyProtectApi *IbmKeyProtectApiV2) GetImportTokenWithContext(ctx context.Context, getImportTokenOptions *GetImportTokenOptions) (result *GetImportToken, response *core.DetailedResponse, err error) {
	err = core.ValidateNotNil(getImportTokenOptions, "getImportTokenOptions cannot be nil")
	if err != nil {
		err = core.SDKErrorf(err, "", "unexpected-nil-param", common.GetComponentInfo())
		return
	}
	err = core.ValidateStruct(getImportTokenOptions, "getImportTokenOptions")
	if err != nil {
		err = core.SDKErrorf(err, "", "struct-validation-error", common.GetComponentInfo())
		return
	}

	builder := core.NewRequestBuilder(core.GET)
	builder = builder.WithContext(ctx)
	builder.EnableGzipCompression = ibmKeyProtectApi.GetEnableGzipCompression()
	_, err = builder.ResolveRequestURL(ibmKeyProtectApi.Service.Options.URL, `/api/v2/import_token`, nil)
	if err != nil {
		err = core.SDKErrorf(err, "", "url-resolve-error", common.GetComponentInfo())
		return
	}

	sdkHeaders := common.GetSdkHeaders("ibm_key_protect_api", "V2", "GetImportToken")
	for headerName, headerValue := range sdkHeaders {
		builder.AddHeader(headerName, headerValue)
	}

	for headerName, headerValue := range getImportTokenOptions.Headers {
		builder.AddHeader(headerName, headerValue)
	}
	builder.AddHeader("Accept", "application/json")
	if getImportTokenOptions.BluemixInstance != nil {
		builder.AddHeader("Bluemix-Instance", fmt.Sprint(*getImportTokenOptions.BluemixInstance))
	}
	if getImportTokenOptions.CorrelationID != nil {
		builder.AddHeader("Correlation-Id", fmt.Sprint(*getImportTokenOptions.CorrelationID))
	}
	if getImportTokenOptions.XKmsKeyRing != nil {
		builder.AddHeader("X-Kms-Key-Ring", fmt.Sprint(*getImportTokenOptions.XKmsKeyRing))
	}

	request, err := builder.Build()
	if err != nil {
		err = core.SDKErrorf(err, "", "build-error", common.GetComponentInfo())
		return
	}

	var rawResponse map[string]json.RawMessage
	response, err = ibmKeyProtectApi.Service.Request(request, &rawResponse)
	if err != nil {
		core.EnrichHTTPProblem(err, "getImportToken", getServiceComponentInfo())
		err = core.SDKErrorf(err, "", "http-request-err", common.GetComponentInfo())
		return
	}
	if rawResponse != nil {
		err = core.UnmarshalModel(rawResponse, "", &result, UnmarshalGetImportToken)
		if err != nil {
			err = core.SDKErrorf(err, "", "unmarshal-resp-error", common.GetComponentInfo())
			return
		}
		response.Result = result
	}

	return
}

// GetRegistrations : List registrations for a key
// Retrieves a list of registrations that are associated with a specified root key.
//
// When you use a root key to protect an IBM Cloud resource, such as a Cloud Object Storage bucket, Key Protect creates
// a registration between the resource and root key. You can use `GET /keys/{id}/registrations` to understand which
// cloud resources are protected by the key that you specify.
func (ibmKeyProtectApi *IbmKeyProtectApiV2) GetRegistrations(getRegistrationsOptions *GetRegistrationsOptions) (result *RegistrationWithTotalCount, response *core.DetailedResponse, err error) {
	result, response, err = ibmKeyProtectApi.GetRegistrationsWithContext(context.Background(), getRegistrationsOptions)
	err = core.RepurposeSDKProblem(err, "")
	return
}

// GetRegistrationsWithContext is an alternate form of the GetRegistrations method which supports a Context parameter
func (ibmKeyProtectApi *IbmKeyProtectApiV2) GetRegistrationsWithContext(ctx context.Context, getRegistrationsOptions *GetRegistrationsOptions) (result *RegistrationWithTotalCount, response *core.DetailedResponse, err error) {
	err = core.ValidateNotNil(getRegistrationsOptions, "getRegistrationsOptions cannot be nil")
	if err != nil {
		err = core.SDKErrorf(err, "", "unexpected-nil-param", common.GetComponentInfo())
		return
	}
	err = core.ValidateStruct(getRegistrationsOptions, "getRegistrationsOptions")
	if err != nil {
		err = core.SDKErrorf(err, "", "struct-validation-error", common.GetComponentInfo())
		return
	}

	pathParamsMap := map[string]string{
		"id": *getRegistrationsOptions.ID,
	}

	builder := core.NewRequestBuilder(core.GET)
	builder = builder.WithContext(ctx)
	builder.EnableGzipCompression = ibmKeyProtectApi.GetEnableGzipCompression()
	_, err = builder.ResolveRequestURL(ibmKeyProtectApi.Service.Options.URL, `/api/v2/keys/{id}/registrations`, pathParamsMap)
	if err != nil {
		err = core.SDKErrorf(err, "", "url-resolve-error", common.GetComponentInfo())
		return
	}

	sdkHeaders := common.GetSdkHeaders("ibm_key_protect_api", "V2", "GetRegistrations")
	for headerName, headerValue := range sdkHeaders {
		builder.AddHeader(headerName, headerValue)
	}

	for headerName, headerValue := range getRegistrationsOptions.Headers {
		builder.AddHeader(headerName, headerValue)
	}
	builder.AddHeader("Accept", "application/json")
	if getRegistrationsOptions.BluemixInstance != nil {
		builder.AddHeader("Bluemix-Instance", fmt.Sprint(*getRegistrationsOptions.BluemixInstance))
	}
	if getRegistrationsOptions.CorrelationID != nil {
		builder.AddHeader("Correlation-Id", fmt.Sprint(*getRegistrationsOptions.CorrelationID))
	}
	if getRegistrationsOptions.XKmsKeyRing != nil {
		builder.AddHeader("X-Kms-Key-Ring", fmt.Sprint(*getRegistrationsOptions.XKmsKeyRing))
	}

	if getRegistrationsOptions.Limit != nil {
		builder.AddQuery("limit", fmt.Sprint(*getRegistrationsOptions.Limit))
	}
	if getRegistrationsOptions.Offset != nil {
		builder.AddQuery("offset", fmt.Sprint(*getRegistrationsOptions.Offset))
	}
	if getRegistrationsOptions.UrlEncodedResourceCRNQuery != nil {
		builder.AddQuery("urlEncodedResourceCRNQuery", fmt.Sprint(*getRegistrationsOptions.UrlEncodedResourceCRNQuery))
	}
	if getRegistrationsOptions.PreventKeyDeletion != nil {
		builder.AddQuery("preventKeyDeletion", fmt.Sprint(*getRegistrationsOptions.PreventKeyDeletion))
	}
	if getRegistrationsOptions.TotalCount != nil {
		builder.AddQuery("totalCount", fmt.Sprint(*getRegistrationsOptions.TotalCount))
	}

	request, err := builder.Build()
	if err != nil {
		err = core.SDKErrorf(err, "", "build-error", common.GetComponentInfo())
		return
	}

	var rawResponse map[string]json.RawMessage
	response, err = ibmKeyProtectApi.Service.Request(request, &rawResponse)
	if err != nil {
		core.EnrichHTTPProblem(err, "getRegistrations", getServiceComponentInfo())
		err = core.SDKErrorf(err, "", "http-request-err", common.GetComponentInfo())
		return
	}
	if rawResponse != nil {
		err = core.UnmarshalModel(rawResponse, "", &result, UnmarshalRegistrationWithTotalCount)
		if err != nil {
			err = core.SDKErrorf(err, "", "unmarshal-resp-error", common.GetComponentInfo())
			return
		}
		response.Result = result
	}

	return
}

// GetRegistrationsAllKeys : List registrations for any key
// Retrieves a list of registrations that match the Cloud Resource Name
// (CRN) query that you specify.
//
// When you use a root key to protect an IBM Cloud resource, such as a Cloud Object Storage bucket, Key Protect creates
// a registration between the resource and root key. You can use `GET /keys/registrations` to understand which cloud
// resources are protected by keys in your Key Protect service instance.
func (ibmKeyProtectApi *IbmKeyProtectApiV2) GetRegistrationsAllKeys(getRegistrationsAllKeysOptions *GetRegistrationsAllKeysOptions) (result *RegistrationWithTotalCount, response *core.DetailedResponse, err error) {
	result, response, err = ibmKeyProtectApi.GetRegistrationsAllKeysWithContext(context.Background(), getRegistrationsAllKeysOptions)
	err = core.RepurposeSDKProblem(err, "")
	return
}

// GetRegistrationsAllKeysWithContext is an alternate form of the GetRegistrationsAllKeys method which supports a Context parameter
func (ibmKeyProtectApi *IbmKeyProtectApiV2) GetRegistrationsAllKeysWithContext(ctx context.Context, getRegistrationsAllKeysOptions *GetRegistrationsAllKeysOptions) (result *RegistrationWithTotalCount, response *core.DetailedResponse, err error) {
	err = core.ValidateNotNil(getRegistrationsAllKeysOptions, "getRegistrationsAllKeysOptions cannot be nil")
	if err != nil {
		err = core.SDKErrorf(err, "", "unexpected-nil-param", common.GetComponentInfo())
		return
	}
	err = core.ValidateStruct(getRegistrationsAllKeysOptions, "getRegistrationsAllKeysOptions")
	if err != nil {
		err = core.SDKErrorf(err, "", "struct-validation-error", common.GetComponentInfo())
		return
	}

	builder := core.NewRequestBuilder(core.GET)
	builder = builder.WithContext(ctx)
	builder.EnableGzipCompression = ibmKeyProtectApi.GetEnableGzipCompression()
	_, err = builder.ResolveRequestURL(ibmKeyProtectApi.Service.Options.URL, `/api/v2/keys/registrations`, nil)
	if err != nil {
		err = core.SDKErrorf(err, "", "url-resolve-error", common.GetComponentInfo())
		return
	}

	sdkHeaders := common.GetSdkHeaders("ibm_key_protect_api", "V2", "GetRegistrationsAllKeys")
	for headerName, headerValue := range sdkHeaders {
		builder.AddHeader(headerName, headerValue)
	}

	for headerName, headerValue := range getRegistrationsAllKeysOptions.Headers {
		builder.AddHeader(headerName, headerValue)
	}
	builder.AddHeader("Accept", "application/json")
	if getRegistrationsAllKeysOptions.BluemixInstance != nil {
		builder.AddHeader("Bluemix-Instance", fmt.Sprint(*getRegistrationsAllKeysOptions.BluemixInstance))
	}
	if getRegistrationsAllKeysOptions.CorrelationID != nil {
		builder.AddHeader("Correlation-Id", fmt.Sprint(*getRegistrationsAllKeysOptions.CorrelationID))
	}
	if getRegistrationsAllKeysOptions.XKmsKeyRing != nil {
		builder.AddHeader("X-Kms-Key-Ring", fmt.Sprint(*getRegistrationsAllKeysOptions.XKmsKeyRing))
	}

	if getRegistrationsAllKeysOptions.UrlEncodedResourceCRNQuery != nil {
		builder.AddQuery("urlEncodedResourceCRNQuery", fmt.Sprint(*getRegistrationsAllKeysOptions.UrlEncodedResourceCRNQuery))
	}
	if getRegistrationsAllKeysOptions.Limit != nil {
		builder.AddQuery("limit", fmt.Sprint(*getRegistrationsAllKeysOptions.Limit))
	}
	if getRegistrationsAllKeysOptions.Offset != nil {
		builder.AddQuery("offset", fmt.Sprint(*getRegistrationsAllKeysOptions.Offset))
	}
	if getRegistrationsAllKeysOptions.PreventKeyDeletion != nil {
		builder.AddQuery("preventKeyDeletion", fmt.Sprint(*getRegistrationsAllKeysOptions.PreventKeyDeletion))
	}
	if getRegistrationsAllKeysOptions.TotalCount != nil {
		builder.AddQuery("totalCount", fmt.Sprint(*getRegistrationsAllKeysOptions.TotalCount))
	}

	request, err := builder.Build()
	if err != nil {
		err = core.SDKErrorf(err, "", "build-error", common.GetComponentInfo())
		return
	}

	var rawResponse map[string]json.RawMessage
	response, err = ibmKeyProtectApi.Service.Request(request, &rawResponse)
	if err != nil {
		core.EnrichHTTPProblem(err, "getRegistrationsAllKeys", getServiceComponentInfo())
		err = core.SDKErrorf(err, "", "http-request-err", common.GetComponentInfo())
		return
	}
	if rawResponse != nil {
		err = core.UnmarshalModel(rawResponse, "", &result, UnmarshalRegistrationWithTotalCount)
		if err != nil {
			err = core.SDKErrorf(err, "", "unmarshal-resp-error", common.GetComponentInfo())
			return
		}
		response.Result = result
	}

	return
}

// CreateKeyAlias : Create an alias
// Creates a unique alias for the specified key.
func (ibmKeyProtectApi *IbmKeyProtectApiV2) CreateKeyAlias(createKeyAliasOptions *CreateKeyAliasOptions) (result *KeyAlias, response *core.DetailedResponse, err error) {
	result, response, err = ibmKeyProtectApi.CreateKeyAliasWithContext(context.Background(), createKeyAliasOptions)
	err = core.RepurposeSDKProblem(err, "")
	return
}

// CreateKeyAliasWithContext is an alternate form of the CreateKeyAlias method which supports a Context parameter
func (ibmKeyProtectApi *IbmKeyProtectApiV2) CreateKeyAliasWithContext(ctx context.Context, createKeyAliasOptions *CreateKeyAliasOptions) (result *KeyAlias, response *core.DetailedResponse, err error) {
	err = core.ValidateNotNil(createKeyAliasOptions, "createKeyAliasOptions cannot be nil")
	if err != nil {
		err = core.SDKErrorf(err, "", "unexpected-nil-param", common.GetComponentInfo())
		return
	}
	err = core.ValidateStruct(createKeyAliasOptions, "createKeyAliasOptions")
	if err != nil {
		err = core.SDKErrorf(err, "", "struct-validation-error", common.GetComponentInfo())
		return
	}

	pathParamsMap := map[string]string{
		"id":    *createKeyAliasOptions.ID,
		"alias": *createKeyAliasOptions.Alias,
	}

	builder := core.NewRequestBuilder(core.POST)
	builder = builder.WithContext(ctx)
	builder.EnableGzipCompression = ibmKeyProtectApi.GetEnableGzipCompression()
	_, err = builder.ResolveRequestURL(ibmKeyProtectApi.Service.Options.URL, `/api/v2/keys/{id}/aliases/{alias}`, pathParamsMap)
	if err != nil {
		err = core.SDKErrorf(err, "", "url-resolve-error", common.GetComponentInfo())
		return
	}

	sdkHeaders := common.GetSdkHeaders("ibm_key_protect_api", "V2", "CreateKeyAlias")
	for headerName, headerValue := range sdkHeaders {
		builder.AddHeader(headerName, headerValue)
	}

	for headerName, headerValue := range createKeyAliasOptions.Headers {
		builder.AddHeader(headerName, headerValue)
	}
	builder.AddHeader("Accept", "application/json")
	if createKeyAliasOptions.BluemixInstance != nil {
		builder.AddHeader("Bluemix-Instance", fmt.Sprint(*createKeyAliasOptions.BluemixInstance))
	}
	if createKeyAliasOptions.CorrelationID != nil {
		builder.AddHeader("Correlation-Id", fmt.Sprint(*createKeyAliasOptions.CorrelationID))
	}
	if createKeyAliasOptions.XKmsKeyRing != nil {
		builder.AddHeader("X-Kms-Key-Ring", fmt.Sprint(*createKeyAliasOptions.XKmsKeyRing))
	}

	request, err := builder.Build()
	if err != nil {
		err = core.SDKErrorf(err, "", "build-error", common.GetComponentInfo())
		return
	}

	var rawResponse map[string]json.RawMessage
	response, err = ibmKeyProtectApi.Service.Request(request, &rawResponse)
	if err != nil {
		core.EnrichHTTPProblem(err, "createKeyAlias", getServiceComponentInfo())
		err = core.SDKErrorf(err, "", "http-request-err", common.GetComponentInfo())
		return
	}
	if rawResponse != nil {
		err = core.UnmarshalModel(rawResponse, "", &result, UnmarshalKeyAlias)
		if err != nil {
			err = core.SDKErrorf(err, "", "unmarshal-resp-error", common.GetComponentInfo())
			return
		}
		response.Result = result
	}

	return
}

// DeleteKeyAlias : Delete an alias
// Deletes an alias from the associated key.
//
// Delete alias does not delete the key.
func (ibmKeyProtectApi *IbmKeyProtectApiV2) DeleteKeyAlias(deleteKeyAliasOptions *DeleteKeyAliasOptions) (response *core.DetailedResponse, err error) {
	response, err = ibmKeyProtectApi.DeleteKeyAliasWithContext(context.Background(), deleteKeyAliasOptions)
	err = core.RepurposeSDKProblem(err, "")
	return
}

// DeleteKeyAliasWithContext is an alternate form of the DeleteKeyAlias method which supports a Context parameter
func (ibmKeyProtectApi *IbmKeyProtectApiV2) DeleteKeyAliasWithContext(ctx context.Context, deleteKeyAliasOptions *DeleteKeyAliasOptions) (response *core.DetailedResponse, err error) {
	err = core.ValidateNotNil(deleteKeyAliasOptions, "deleteKeyAliasOptions cannot be nil")
	if err != nil {
		err = core.SDKErrorf(err, "", "unexpected-nil-param", common.GetComponentInfo())
		return
	}
	err = core.ValidateStruct(deleteKeyAliasOptions, "deleteKeyAliasOptions")
	if err != nil {
		err = core.SDKErrorf(err, "", "struct-validation-error", common.GetComponentInfo())
		return
	}

	pathParamsMap := map[string]string{
		"id":    *deleteKeyAliasOptions.ID,
		"alias": *deleteKeyAliasOptions.Alias,
	}

	builder := core.NewRequestBuilder(core.DELETE)
	builder = builder.WithContext(ctx)
	builder.EnableGzipCompression = ibmKeyProtectApi.GetEnableGzipCompression()
	_, err = builder.ResolveRequestURL(ibmKeyProtectApi.Service.Options.URL, `/api/v2/keys/{id}/aliases/{alias}`, pathParamsMap)
	if err != nil {
		err = core.SDKErrorf(err, "", "url-resolve-error", common.GetComponentInfo())
		return
	}

	sdkHeaders := common.GetSdkHeaders("ibm_key_protect_api", "V2", "DeleteKeyAlias")
	for headerName, headerValue := range sdkHeaders {
		builder.AddHeader(headerName, headerValue)
	}

	for headerName, headerValue := range deleteKeyAliasOptions.Headers {
		builder.AddHeader(headerName, headerValue)
	}
	if deleteKeyAliasOptions.BluemixInstance != nil {
		builder.AddHeader("Bluemix-Instance", fmt.Sprint(*deleteKeyAliasOptions.BluemixInstance))
	}
	if deleteKeyAliasOptions.CorrelationID != nil {
		builder.AddHeader("Correlation-Id", fmt.Sprint(*deleteKeyAliasOptions.CorrelationID))
	}
	if deleteKeyAliasOptions.XKmsKeyRing != nil {
		builder.AddHeader("X-Kms-Key-Ring", fmt.Sprint(*deleteKeyAliasOptions.XKmsKeyRing))
	}

	request, err := builder.Build()
	if err != nil {
		err = core.SDKErrorf(err, "", "build-error", common.GetComponentInfo())
		return
	}

	response, err = ibmKeyProtectApi.Service.Request(request, nil)
	if err != nil {
		core.EnrichHTTPProblem(err, "deleteKeyAlias", getServiceComponentInfo())
		err = core.SDKErrorf(err, "", "http-request-err", common.GetComponentInfo())
		return
	}

	return
}

// ListKeyRings : List key rings
// List all key rings in the instance.
func (ibmKeyProtectApi *IbmKeyProtectApiV2) ListKeyRings(listKeyRingsOptions *ListKeyRingsOptions) (result *ListKeyRingsWithTotalCount, response *core.DetailedResponse, err error) {
	result, response, err = ibmKeyProtectApi.ListKeyRingsWithContext(context.Background(), listKeyRingsOptions)
	err = core.RepurposeSDKProblem(err, "")
	return
}

// ListKeyRingsWithContext is an alternate form of the ListKeyRings method which supports a Context parameter
func (ibmKeyProtectApi *IbmKeyProtectApiV2) ListKeyRingsWithContext(ctx context.Context, listKeyRingsOptions *ListKeyRingsOptions) (result *ListKeyRingsWithTotalCount, response *core.DetailedResponse, err error) {
	err = core.ValidateNotNil(listKeyRingsOptions, "listKeyRingsOptions cannot be nil")
	if err != nil {
		err = core.SDKErrorf(err, "", "unexpected-nil-param", common.GetComponentInfo())
		return
	}
	err = core.ValidateStruct(listKeyRingsOptions, "listKeyRingsOptions")
	if err != nil {
		err = core.SDKErrorf(err, "", "struct-validation-error", common.GetComponentInfo())
		return
	}

	builder := core.NewRequestBuilder(core.GET)
	builder = builder.WithContext(ctx)
	builder.EnableGzipCompression = ibmKeyProtectApi.GetEnableGzipCompression()
	_, err = builder.ResolveRequestURL(ibmKeyProtectApi.Service.Options.URL, `/api/v2/key_rings`, nil)
	if err != nil {
		err = core.SDKErrorf(err, "", "url-resolve-error", common.GetComponentInfo())
		return
	}

	sdkHeaders := common.GetSdkHeaders("ibm_key_protect_api", "V2", "ListKeyRings")
	for headerName, headerValue := range sdkHeaders {
		builder.AddHeader(headerName, headerValue)
	}

	for headerName, headerValue := range listKeyRingsOptions.Headers {
		builder.AddHeader(headerName, headerValue)
	}
	builder.AddHeader("Accept", "application/json")
	if listKeyRingsOptions.BluemixInstance != nil {
		builder.AddHeader("Bluemix-Instance", fmt.Sprint(*listKeyRingsOptions.BluemixInstance))
	}
	if listKeyRingsOptions.CorrelationID != nil {
		builder.AddHeader("Correlation-Id", fmt.Sprint(*listKeyRingsOptions.CorrelationID))
	}

	if listKeyRingsOptions.Limit != nil {
		builder.AddQuery("limit", fmt.Sprint(*listKeyRingsOptions.Limit))
	}
	if listKeyRingsOptions.Offset != nil {
		builder.AddQuery("offset", fmt.Sprint(*listKeyRingsOptions.Offset))
	}
	if listKeyRingsOptions.TotalCount != nil {
		builder.AddQuery("totalCount", fmt.Sprint(*listKeyRingsOptions.TotalCount))
	}

	request, err := builder.Build()
	if err != nil {
		err = core.SDKErrorf(err, "", "build-error", common.GetComponentInfo())
		return
	}

	var rawResponse map[string]json.RawMessage
	response, err = ibmKeyProtectApi.Service.Request(request, &rawResponse)
	if err != nil {
		core.EnrichHTTPProblem(err, "listKeyRings", getServiceComponentInfo())
		err = core.SDKErrorf(err, "", "http-request-err", common.GetComponentInfo())
		return
	}
	if rawResponse != nil {
		err = core.UnmarshalModel(rawResponse, "", &result, UnmarshalListKeyRingsWithTotalCount)
		if err != nil {
			err = core.SDKErrorf(err, "", "unmarshal-resp-error", common.GetComponentInfo())
			return
		}
		response.Result = result
	}

	return
}

// CreateKeyRing : Create a key ring
// Create a key ring in the instance with the specified name. The key ring ID `default` is a reserved key ring ID and
// cannot be created nor destroyed. The `default` key ring is an initial key ring that is generated with each newly
// created instance. All keys not associated with an otherwise specified key ring exist within the default key ring.
func (ibmKeyProtectApi *IbmKeyProtectApiV2) CreateKeyRing(createKeyRingOptions *CreateKeyRingOptions) (response *core.DetailedResponse, err error) {
	response, err = ibmKeyProtectApi.CreateKeyRingWithContext(context.Background(), createKeyRingOptions)
	err = core.RepurposeSDKProblem(err, "")
	return
}

// CreateKeyRingWithContext is an alternate form of the CreateKeyRing method which supports a Context parameter
func (ibmKeyProtectApi *IbmKeyProtectApiV2) CreateKeyRingWithContext(ctx context.Context, createKeyRingOptions *CreateKeyRingOptions) (response *core.DetailedResponse, err error) {
	err = core.ValidateNotNil(createKeyRingOptions, "createKeyRingOptions cannot be nil")
	if err != nil {
		err = core.SDKErrorf(err, "", "unexpected-nil-param", common.GetComponentInfo())
		return
	}
	err = core.ValidateStruct(createKeyRingOptions, "createKeyRingOptions")
	if err != nil {
		err = core.SDKErrorf(err, "", "struct-validation-error", common.GetComponentInfo())
		return
	}

	pathParamsMap := map[string]string{
		"key-ring-id": *createKeyRingOptions.KeyRingID,
	}

	builder := core.NewRequestBuilder(core.POST)
	builder = builder.WithContext(ctx)
	builder.EnableGzipCompression = ibmKeyProtectApi.GetEnableGzipCompression()
	_, err = builder.ResolveRequestURL(ibmKeyProtectApi.Service.Options.URL, `/api/v2/key_rings/{key-ring-id}`, pathParamsMap)
	if err != nil {
		err = core.SDKErrorf(err, "", "url-resolve-error", common.GetComponentInfo())
		return
	}

	sdkHeaders := common.GetSdkHeaders("ibm_key_protect_api", "V2", "CreateKeyRing")
	for headerName, headerValue := range sdkHeaders {
		builder.AddHeader(headerName, headerValue)
	}

	for headerName, headerValue := range createKeyRingOptions.Headers {
		builder.AddHeader(headerName, headerValue)
	}
	if createKeyRingOptions.BluemixInstance != nil {
		builder.AddHeader("Bluemix-Instance", fmt.Sprint(*createKeyRingOptions.BluemixInstance))
	}
	if createKeyRingOptions.CorrelationID != nil {
		builder.AddHeader("Correlation-Id", fmt.Sprint(*createKeyRingOptions.CorrelationID))
	}

	request, err := builder.Build()
	if err != nil {
		err = core.SDKErrorf(err, "", "build-error", common.GetComponentInfo())
		return
	}

	response, err = ibmKeyProtectApi.Service.Request(request, nil)
	if err != nil {
		core.EnrichHTTPProblem(err, "createKeyRing", getServiceComponentInfo())
		err = core.SDKErrorf(err, "", "http-request-err", common.GetComponentInfo())
		return
	}

	return
}

// DeleteKeyRing : Delete key ring
// Delete the key ring from the instance. The key ring ID `default` cannot be destroyed.
func (ibmKeyProtectApi *IbmKeyProtectApiV2) DeleteKeyRing(deleteKeyRingOptions *DeleteKeyRingOptions) (response *core.DetailedResponse, err error) {
	response, err = ibmKeyProtectApi.DeleteKeyRingWithContext(context.Background(), deleteKeyRingOptions)
	err = core.RepurposeSDKProblem(err, "")
	return
}

// DeleteKeyRingWithContext is an alternate form of the DeleteKeyRing method which supports a Context parameter
func (ibmKeyProtectApi *IbmKeyProtectApiV2) DeleteKeyRingWithContext(ctx context.Context, deleteKeyRingOptions *DeleteKeyRingOptions) (response *core.DetailedResponse, err error) {
	err = core.ValidateNotNil(deleteKeyRingOptions, "deleteKeyRingOptions cannot be nil")
	if err != nil {
		err = core.SDKErrorf(err, "", "unexpected-nil-param", common.GetComponentInfo())
		return
	}
	err = core.ValidateStruct(deleteKeyRingOptions, "deleteKeyRingOptions")
	if err != nil {
		err = core.SDKErrorf(err, "", "struct-validation-error", common.GetComponentInfo())
		return
	}

	pathParamsMap := map[string]string{
		"key-ring-id": *deleteKeyRingOptions.KeyRingID,
	}

	builder := core.NewRequestBuilder(core.DELETE)
	builder = builder.WithContext(ctx)
	builder.EnableGzipCompression = ibmKeyProtectApi.GetEnableGzipCompression()
	_, err = builder.ResolveRequestURL(ibmKeyProtectApi.Service.Options.URL, `/api/v2/key_rings/{key-ring-id}`, pathParamsMap)
	if err != nil {
		err = core.SDKErrorf(err, "", "url-resolve-error", common.GetComponentInfo())
		return
	}

	sdkHeaders := common.GetSdkHeaders("ibm_key_protect_api", "V2", "DeleteKeyRing")
	for headerName, headerValue := range sdkHeaders {
		builder.AddHeader(headerName, headerValue)
	}

	for headerName, headerValue := range deleteKeyRingOptions.Headers {
		builder.AddHeader(headerName, headerValue)
	}
	if deleteKeyRingOptions.BluemixInstance != nil {
		builder.AddHeader("Bluemix-Instance", fmt.Sprint(*deleteKeyRingOptions.BluemixInstance))
	}
	if deleteKeyRingOptions.CorrelationID != nil {
		builder.AddHeader("Correlation-Id", fmt.Sprint(*deleteKeyRingOptions.CorrelationID))
	}

	if deleteKeyRingOptions.Force != nil {
		builder.AddQuery("force", fmt.Sprint(*deleteKeyRingOptions.Force))
	}

	request, err := builder.Build()
	if err != nil {
		err = core.SDKErrorf(err, "", "build-error", common.GetComponentInfo())
		return
	}

	response, err = ibmKeyProtectApi.Service.Request(request, nil)
	if err != nil {
		core.EnrichHTTPProblem(err, "deleteKeyRing", getServiceComponentInfo())
		err = core.SDKErrorf(err, "", "http-request-err", common.GetComponentInfo())
		return
	}

	return
}

// GetKmipAdapters : List KMIP Adapters
// Retrieves a list of KMIP Adapters.
func (ibmKeyProtectApi *IbmKeyProtectApiV2) GetKmipAdapters(getKmipAdaptersOptions *GetKmipAdaptersOptions) (result *ListKMIPAdaptersWithTotalCount, response *core.DetailedResponse, err error) {
	result, response, err = ibmKeyProtectApi.GetKmipAdaptersWithContext(context.Background(), getKmipAdaptersOptions)
	err = core.RepurposeSDKProblem(err, "")
	return
}

// GetKmipAdaptersWithContext is an alternate form of the GetKmipAdapters method which supports a Context parameter
func (ibmKeyProtectApi *IbmKeyProtectApiV2) GetKmipAdaptersWithContext(ctx context.Context, getKmipAdaptersOptions *GetKmipAdaptersOptions) (result *ListKMIPAdaptersWithTotalCount, response *core.DetailedResponse, err error) {
	err = core.ValidateNotNil(getKmipAdaptersOptions, "getKmipAdaptersOptions cannot be nil")
	if err != nil {
		err = core.SDKErrorf(err, "", "unexpected-nil-param", common.GetComponentInfo())
		return
	}
	err = core.ValidateStruct(getKmipAdaptersOptions, "getKmipAdaptersOptions")
	if err != nil {
		err = core.SDKErrorf(err, "", "struct-validation-error", common.GetComponentInfo())
		return
	}

	builder := core.NewRequestBuilder(core.GET)
	builder = builder.WithContext(ctx)
	builder.EnableGzipCompression = ibmKeyProtectApi.GetEnableGzipCompression()
	_, err = builder.ResolveRequestURL(ibmKeyProtectApi.Service.Options.URL, `/api/v2/kmip_adapters`, nil)
	if err != nil {
		err = core.SDKErrorf(err, "", "url-resolve-error", common.GetComponentInfo())
		return
	}

	sdkHeaders := common.GetSdkHeaders("ibm_key_protect_api", "V2", "GetKmipAdapters")
	for headerName, headerValue := range sdkHeaders {
		builder.AddHeader(headerName, headerValue)
	}

	for headerName, headerValue := range getKmipAdaptersOptions.Headers {
		builder.AddHeader(headerName, headerValue)
	}
	builder.AddHeader("Accept", "application/json")
	if getKmipAdaptersOptions.BluemixInstance != nil {
		builder.AddHeader("Bluemix-Instance", fmt.Sprint(*getKmipAdaptersOptions.BluemixInstance))
	}
	if getKmipAdaptersOptions.CorrelationID != nil {
		builder.AddHeader("Correlation-Id", fmt.Sprint(*getKmipAdaptersOptions.CorrelationID))
	}

	if getKmipAdaptersOptions.Limit != nil {
		builder.AddQuery("limit", fmt.Sprint(*getKmipAdaptersOptions.Limit))
	}
	if getKmipAdaptersOptions.Offset != nil {
		builder.AddQuery("offset", fmt.Sprint(*getKmipAdaptersOptions.Offset))
	}
	if getKmipAdaptersOptions.TotalCount != nil {
		builder.AddQuery("totalCount", fmt.Sprint(*getKmipAdaptersOptions.TotalCount))
	}
	if getKmipAdaptersOptions.CrkID != nil {
		builder.AddQuery("crk_id", fmt.Sprint(*getKmipAdaptersOptions.CrkID))
	}

	request, err := builder.Build()
	if err != nil {
		err = core.SDKErrorf(err, "", "build-error", common.GetComponentInfo())
		return
	}

	var rawResponse map[string]json.RawMessage
	response, err = ibmKeyProtectApi.Service.Request(request, &rawResponse)
	if err != nil {
		core.EnrichHTTPProblem(err, "get_kmip_adapters", getServiceComponentInfo())
		err = core.SDKErrorf(err, "", "http-request-err", common.GetComponentInfo())
		return
	}
	if rawResponse != nil {
		err = core.UnmarshalModel(rawResponse, "", &result, UnmarshalListKMIPAdaptersWithTotalCount)
		if err != nil {
			err = core.SDKErrorf(err, "", "unmarshal-resp-error", common.GetComponentInfo())
			return
		}
		response.Result = result
	}

	return
}

// CreateKmipAdapter : Create a KMIP Adapter
// Creates a KMIP adapter.
func (ibmKeyProtectApi *IbmKeyProtectApiV2) CreateKmipAdapter(createKmipAdapterOptions *CreateKmipAdapterOptions) (result *ListKMIPAdapters, response *core.DetailedResponse, err error) {
	result, response, err = ibmKeyProtectApi.CreateKmipAdapterWithContext(context.Background(), createKmipAdapterOptions)
	err = core.RepurposeSDKProblem(err, "")
	return
}

// CreateKmipAdapterWithContext is an alternate form of the CreateKmipAdapter method which supports a Context parameter
func (ibmKeyProtectApi *IbmKeyProtectApiV2) CreateKmipAdapterWithContext(ctx context.Context, createKmipAdapterOptions *CreateKmipAdapterOptions) (result *ListKMIPAdapters, response *core.DetailedResponse, err error) {
	err = core.ValidateNotNil(createKmipAdapterOptions, "createKmipAdapterOptions cannot be nil")
	if err != nil {
		err = core.SDKErrorf(err, "", "unexpected-nil-param", common.GetComponentInfo())
		return
	}
	err = core.ValidateStruct(createKmipAdapterOptions, "createKmipAdapterOptions")
	if err != nil {
		err = core.SDKErrorf(err, "", "struct-validation-error", common.GetComponentInfo())
		return
	}

	builder := core.NewRequestBuilder(core.POST)
	builder = builder.WithContext(ctx)
	builder.EnableGzipCompression = ibmKeyProtectApi.GetEnableGzipCompression()
	_, err = builder.ResolveRequestURL(ibmKeyProtectApi.Service.Options.URL, `/api/v2/kmip_adapters`, nil)
	if err != nil {
		err = core.SDKErrorf(err, "", "url-resolve-error", common.GetComponentInfo())
		return
	}

	sdkHeaders := common.GetSdkHeaders("ibm_key_protect_api", "V2", "CreateKmipAdapter")
	for headerName, headerValue := range sdkHeaders {
		builder.AddHeader(headerName, headerValue)
	}

	for headerName, headerValue := range createKmipAdapterOptions.Headers {
		builder.AddHeader(headerName, headerValue)
	}
	builder.AddHeader("Accept", "application/json")
	builder.AddHeader("Content-Type", "application/json")
	if createKmipAdapterOptions.BluemixInstance != nil {
		builder.AddHeader("Bluemix-Instance", fmt.Sprint(*createKmipAdapterOptions.BluemixInstance))
	}
	if createKmipAdapterOptions.CorrelationID != nil {
		builder.AddHeader("Correlation-Id", fmt.Sprint(*createKmipAdapterOptions.CorrelationID))
	}

	if createKmipAdapterOptions.AllowExpiringKey != nil {
		builder.AddQuery("allowExpiringKey", fmt.Sprint(*createKmipAdapterOptions.AllowExpiringKey))
	}

	body := make(map[string]interface{})
	if createKmipAdapterOptions.Metadata != nil {
		body["metadata"] = createKmipAdapterOptions.Metadata
	}
	if createKmipAdapterOptions.Resources != nil {
		body["resources"] = createKmipAdapterOptions.Resources
	}
	_, err = builder.SetBodyContentJSON(body)
	if err != nil {
		err = core.SDKErrorf(err, "", "set-json-body-error", common.GetComponentInfo())
		return
	}

	request, err := builder.Build()
	if err != nil {
		err = core.SDKErrorf(err, "", "build-error", common.GetComponentInfo())
		return
	}

	var rawResponse map[string]json.RawMessage
	response, err = ibmKeyProtectApi.Service.Request(request, &rawResponse)
	if err != nil {
		core.EnrichHTTPProblem(err, "create_kmip_adapter", getServiceComponentInfo())
		err = core.SDKErrorf(err, "", "http-request-err", common.GetComponentInfo())
		return
	}
	if rawResponse != nil {
		err = core.UnmarshalModel(rawResponse, "", &result, UnmarshalListKMIPAdapters)
		if err != nil {
			err = core.SDKErrorf(err, "", "unmarshal-resp-error", common.GetComponentInfo())
			return
		}
		response.Result = result
	}

	return
}

// GetKmipAdapter : Retrieve a KMIP Adapter
// Retrieves a KMIP adapter using its id / name.
func (ibmKeyProtectApi *IbmKeyProtectApiV2) GetKmipAdapter(getKmipAdapterOptions *GetKmipAdapterOptions) (result *ListKMIPAdapters, response *core.DetailedResponse, err error) {
	result, response, err = ibmKeyProtectApi.GetKmipAdapterWithContext(context.Background(), getKmipAdapterOptions)
	err = core.RepurposeSDKProblem(err, "")
	return
}

// GetKmipAdapterWithContext is an alternate form of the GetKmipAdapter method which supports a Context parameter
func (ibmKeyProtectApi *IbmKeyProtectApiV2) GetKmipAdapterWithContext(ctx context.Context, getKmipAdapterOptions *GetKmipAdapterOptions) (result *ListKMIPAdapters, response *core.DetailedResponse, err error) {
	err = core.ValidateNotNil(getKmipAdapterOptions, "getKmipAdapterOptions cannot be nil")
	if err != nil {
		err = core.SDKErrorf(err, "", "unexpected-nil-param", common.GetComponentInfo())
		return
	}
	err = core.ValidateStruct(getKmipAdapterOptions, "getKmipAdapterOptions")
	if err != nil {
		err = core.SDKErrorf(err, "", "struct-validation-error", common.GetComponentInfo())
		return
	}

	pathParamsMap := map[string]string{
		"id": *getKmipAdapterOptions.ID,
	}

	builder := core.NewRequestBuilder(core.GET)
	builder = builder.WithContext(ctx)
	builder.EnableGzipCompression = ibmKeyProtectApi.GetEnableGzipCompression()
	_, err = builder.ResolveRequestURL(ibmKeyProtectApi.Service.Options.URL, `/api/v2/kmip_adapters/{id}`, pathParamsMap)
	if err != nil {
		err = core.SDKErrorf(err, "", "url-resolve-error", common.GetComponentInfo())
		return
	}

	sdkHeaders := common.GetSdkHeaders("ibm_key_protect_api", "V2", "GetKmipAdapter")
	for headerName, headerValue := range sdkHeaders {
		builder.AddHeader(headerName, headerValue)
	}

	for headerName, headerValue := range getKmipAdapterOptions.Headers {
		builder.AddHeader(headerName, headerValue)
	}
	builder.AddHeader("Accept", "application/json")
	if getKmipAdapterOptions.BluemixInstance != nil {
		builder.AddHeader("Bluemix-Instance", fmt.Sprint(*getKmipAdapterOptions.BluemixInstance))
	}
	if getKmipAdapterOptions.CorrelationID != nil {
		builder.AddHeader("Correlation-Id", fmt.Sprint(*getKmipAdapterOptions.CorrelationID))
	}

	request, err := builder.Build()
	if err != nil {
		err = core.SDKErrorf(err, "", "build-error", common.GetComponentInfo())
		return
	}

	var rawResponse map[string]json.RawMessage
	response, err = ibmKeyProtectApi.Service.Request(request, &rawResponse)
	if err != nil {
		core.EnrichHTTPProblem(err, "get_kmip_adapter", getServiceComponentInfo())
		err = core.SDKErrorf(err, "", "http-request-err", common.GetComponentInfo())
		return
	}
	if rawResponse != nil {
		err = core.UnmarshalModel(rawResponse, "", &result, UnmarshalListKMIPAdapters)
		if err != nil {
			err = core.SDKErrorf(err, "", "unmarshal-resp-error", common.GetComponentInfo())
			return
		}
		response.Result = result
	}

	return
}

// DeleteKmipAdapter : Delete a KMIP Adapter
// Deletes a KMIP Adapter, including all its client certificates, with the given id / name.
func (ibmKeyProtectApi *IbmKeyProtectApiV2) DeleteKmipAdapter(deleteKmipAdapterOptions *DeleteKmipAdapterOptions) (response *core.DetailedResponse, err error) {
	response, err = ibmKeyProtectApi.DeleteKmipAdapterWithContext(context.Background(), deleteKmipAdapterOptions)
	err = core.RepurposeSDKProblem(err, "")
	return
}

// DeleteKmipAdapterWithContext is an alternate form of the DeleteKmipAdapter method which supports a Context parameter
func (ibmKeyProtectApi *IbmKeyProtectApiV2) DeleteKmipAdapterWithContext(ctx context.Context, deleteKmipAdapterOptions *DeleteKmipAdapterOptions) (response *core.DetailedResponse, err error) {
	err = core.ValidateNotNil(deleteKmipAdapterOptions, "deleteKmipAdapterOptions cannot be nil")
	if err != nil {
		err = core.SDKErrorf(err, "", "unexpected-nil-param", common.GetComponentInfo())
		return
	}
	err = core.ValidateStruct(deleteKmipAdapterOptions, "deleteKmipAdapterOptions")
	if err != nil {
		err = core.SDKErrorf(err, "", "struct-validation-error", common.GetComponentInfo())
		return
	}

	pathParamsMap := map[string]string{
		"id": *deleteKmipAdapterOptions.ID,
	}

	builder := core.NewRequestBuilder(core.DELETE)
	builder = builder.WithContext(ctx)
	builder.EnableGzipCompression = ibmKeyProtectApi.GetEnableGzipCompression()
	_, err = builder.ResolveRequestURL(ibmKeyProtectApi.Service.Options.URL, `/api/v2/kmip_adapters/{id}`, pathParamsMap)
	if err != nil {
		err = core.SDKErrorf(err, "", "url-resolve-error", common.GetComponentInfo())
		return
	}

	sdkHeaders := common.GetSdkHeaders("ibm_key_protect_api", "V2", "DeleteKmipAdapter")
	for headerName, headerValue := range sdkHeaders {
		builder.AddHeader(headerName, headerValue)
	}

	for headerName, headerValue := range deleteKmipAdapterOptions.Headers {
		builder.AddHeader(headerName, headerValue)
	}
	if deleteKmipAdapterOptions.BluemixInstance != nil {
		builder.AddHeader("Bluemix-Instance", fmt.Sprint(*deleteKmipAdapterOptions.BluemixInstance))
	}
	if deleteKmipAdapterOptions.CorrelationID != nil {
		builder.AddHeader("Correlation-Id", fmt.Sprint(*deleteKmipAdapterOptions.CorrelationID))
	}

	request, err := builder.Build()
	if err != nil {
		err = core.SDKErrorf(err, "", "build-error", common.GetComponentInfo())
		return
	}

	response, err = ibmKeyProtectApi.Service.Request(request, nil)
	if err != nil {
		core.EnrichHTTPProblem(err, "delete_kmip_adapter", getServiceComponentInfo())
		err = core.SDKErrorf(err, "", "http-request-err", common.GetComponentInfo())
		return
	}

	return
}

// GetKmipObjects : List KMIP objects of a KMIP Adapter
// List KMIP objects of a KMIP Adapter.
func (ibmKeyProtectApi *IbmKeyProtectApiV2) GetKmipObjects(getKmipObjectsOptions *GetKmipObjectsOptions) (result *ListKMIPObjectsWithTotalCount, response *core.DetailedResponse, err error) {
	result, response, err = ibmKeyProtectApi.GetKmipObjectsWithContext(context.Background(), getKmipObjectsOptions)
	err = core.RepurposeSDKProblem(err, "")
	return
}

// GetKmipObjectsWithContext is an alternate form of the GetKmipObjects method which supports a Context parameter
func (ibmKeyProtectApi *IbmKeyProtectApiV2) GetKmipObjectsWithContext(ctx context.Context, getKmipObjectsOptions *GetKmipObjectsOptions) (result *ListKMIPObjectsWithTotalCount, response *core.DetailedResponse, err error) {
	err = core.ValidateNotNil(getKmipObjectsOptions, "getKmipObjectsOptions cannot be nil")
	if err != nil {
		err = core.SDKErrorf(err, "", "unexpected-nil-param", common.GetComponentInfo())
		return
	}
	err = core.ValidateStruct(getKmipObjectsOptions, "getKmipObjectsOptions")
	if err != nil {
		err = core.SDKErrorf(err, "", "struct-validation-error", common.GetComponentInfo())
		return
	}

	pathParamsMap := map[string]string{
		"adapter_id": *getKmipObjectsOptions.AdapterID,
	}

	builder := core.NewRequestBuilder(core.GET)
	builder = builder.WithContext(ctx)
	builder.EnableGzipCompression = ibmKeyProtectApi.GetEnableGzipCompression()
	_, err = builder.ResolveRequestURL(ibmKeyProtectApi.Service.Options.URL, `/api/v2/kmip_adapters/{adapter_id}/kmip_objects`, pathParamsMap)
	if err != nil {
		err = core.SDKErrorf(err, "", "url-resolve-error", common.GetComponentInfo())
		return
	}

	sdkHeaders := common.GetSdkHeaders("ibm_key_protect_api", "V2", "GetKmipObjects")
	for headerName, headerValue := range sdkHeaders {
		builder.AddHeader(headerName, headerValue)
	}

	for headerName, headerValue := range getKmipObjectsOptions.Headers {
		builder.AddHeader(headerName, headerValue)
	}
	builder.AddHeader("Accept", "application/json")
	if getKmipObjectsOptions.BluemixInstance != nil {
		builder.AddHeader("Bluemix-Instance", fmt.Sprint(*getKmipObjectsOptions.BluemixInstance))
	}
	if getKmipObjectsOptions.CorrelationID != nil {
		builder.AddHeader("Correlation-Id", fmt.Sprint(*getKmipObjectsOptions.CorrelationID))
	}

	if getKmipObjectsOptions.Limit != nil {
		builder.AddQuery("limit", fmt.Sprint(*getKmipObjectsOptions.Limit))
	}
	if getKmipObjectsOptions.Offset != nil {
		builder.AddQuery("offset", fmt.Sprint(*getKmipObjectsOptions.Offset))
	}
	if getKmipObjectsOptions.TotalCount != nil {
		builder.AddQuery("totalCount", fmt.Sprint(*getKmipObjectsOptions.TotalCount))
	}
	if getKmipObjectsOptions.State != nil {
		err = builder.AddQuerySlice("state", getKmipObjectsOptions.State)
		if err != nil {
			err = core.SDKErrorf(err, "", "add-query-slice-error", common.GetComponentInfo())
			return
		}
	}

	request, err := builder.Build()
	if err != nil {
		err = core.SDKErrorf(err, "", "build-error", common.GetComponentInfo())
		return
	}

	var rawResponse map[string]json.RawMessage
	response, err = ibmKeyProtectApi.Service.Request(request, &rawResponse)
	if err != nil {
		core.EnrichHTTPProblem(err, "get_kmip_objects", getServiceComponentInfo())
		err = core.SDKErrorf(err, "", "http-request-err", common.GetComponentInfo())
		return
	}
	if rawResponse != nil {
		err = core.UnmarshalModel(rawResponse, "", &result, UnmarshalListKMIPObjectsWithTotalCount)
		if err != nil {
			err = core.SDKErrorf(err, "", "unmarshal-resp-error", common.GetComponentInfo())
			return
		}
		response.Result = result
	}

	return
}

// GetKmipObject : Retrieve a KMIP object from a KMIP Adapter
// Retrieves a KMIP object from a KMIP Adapter by its id.
func (ibmKeyProtectApi *IbmKeyProtectApiV2) GetKmipObject(getKmipObjectOptions *GetKmipObjectOptions) (result *ListKMIPObjectsWithTotalCount, response *core.DetailedResponse, err error) {
	result, response, err = ibmKeyProtectApi.GetKmipObjectWithContext(context.Background(), getKmipObjectOptions)
	err = core.RepurposeSDKProblem(err, "")
	return
}

// GetKmipObjectWithContext is an alternate form of the GetKmipObject method which supports a Context parameter
func (ibmKeyProtectApi *IbmKeyProtectApiV2) GetKmipObjectWithContext(ctx context.Context, getKmipObjectOptions *GetKmipObjectOptions) (result *ListKMIPObjectsWithTotalCount, response *core.DetailedResponse, err error) {
	err = core.ValidateNotNil(getKmipObjectOptions, "getKmipObjectOptions cannot be nil")
	if err != nil {
		err = core.SDKErrorf(err, "", "unexpected-nil-param", common.GetComponentInfo())
		return
	}
	err = core.ValidateStruct(getKmipObjectOptions, "getKmipObjectOptions")
	if err != nil {
		err = core.SDKErrorf(err, "", "struct-validation-error", common.GetComponentInfo())
		return
	}

	pathParamsMap := map[string]string{
		"adapter_id": *getKmipObjectOptions.AdapterID,
		"id":         *getKmipObjectOptions.ID,
	}

	builder := core.NewRequestBuilder(core.GET)
	builder = builder.WithContext(ctx)
	builder.EnableGzipCompression = ibmKeyProtectApi.GetEnableGzipCompression()
	_, err = builder.ResolveRequestURL(ibmKeyProtectApi.Service.Options.URL, `/api/v2/kmip_adapters/{adapter_id}/kmip_objects/{id}`, pathParamsMap)
	if err != nil {
		err = core.SDKErrorf(err, "", "url-resolve-error", common.GetComponentInfo())
		return
	}

	sdkHeaders := common.GetSdkHeaders("ibm_key_protect_api", "V2", "GetKmipObject")
	for headerName, headerValue := range sdkHeaders {
		builder.AddHeader(headerName, headerValue)
	}

	for headerName, headerValue := range getKmipObjectOptions.Headers {
		builder.AddHeader(headerName, headerValue)
	}
	builder.AddHeader("Accept", "application/json")
	if getKmipObjectOptions.BluemixInstance != nil {
		builder.AddHeader("Bluemix-Instance", fmt.Sprint(*getKmipObjectOptions.BluemixInstance))
	}
	if getKmipObjectOptions.CorrelationID != nil {
		builder.AddHeader("Correlation-Id", fmt.Sprint(*getKmipObjectOptions.CorrelationID))
	}

	request, err := builder.Build()
	if err != nil {
		err = core.SDKErrorf(err, "", "build-error", common.GetComponentInfo())
		return
	}

	var rawResponse map[string]json.RawMessage
	response, err = ibmKeyProtectApi.Service.Request(request, &rawResponse)
	if err != nil {
		core.EnrichHTTPProblem(err, "get_kmip_object", getServiceComponentInfo())
		err = core.SDKErrorf(err, "", "http-request-err", common.GetComponentInfo())
		return
	}
	if rawResponse != nil {
		err = core.UnmarshalModel(rawResponse, "", &result, UnmarshalListKMIPObjectsWithTotalCount)
		if err != nil {
			err = core.SDKErrorf(err, "", "unmarshal-resp-error", common.GetComponentInfo())
			return
		}
		response.Result = result
	}

	return
}

// DeleteKmipObject : Delete a KMIP object from a KMIP Adapter
// Deletes a KMIP object from a KMIP Adapter given its id. Changes the state of the KMIP object to 5 (Destroyed) and
// erases its key material. Any data encrypted by this KMIP object will be crypto erased when the KMIP Object changes it
// state to 5 (Destroyed).
func (ibmKeyProtectApi *IbmKeyProtectApiV2) DeleteKmipObject(deleteKmipObjectOptions *DeleteKmipObjectOptions) (response *core.DetailedResponse, err error) {
	response, err = ibmKeyProtectApi.DeleteKmipObjectWithContext(context.Background(), deleteKmipObjectOptions)
	err = core.RepurposeSDKProblem(err, "")
	return
}

// DeleteKmipObjectWithContext is an alternate form of the DeleteKmipObject method which supports a Context parameter
func (ibmKeyProtectApi *IbmKeyProtectApiV2) DeleteKmipObjectWithContext(ctx context.Context, deleteKmipObjectOptions *DeleteKmipObjectOptions) (response *core.DetailedResponse, err error) {
	err = core.ValidateNotNil(deleteKmipObjectOptions, "deleteKmipObjectOptions cannot be nil")
	if err != nil {
		err = core.SDKErrorf(err, "", "unexpected-nil-param", common.GetComponentInfo())
		return
	}
	err = core.ValidateStruct(deleteKmipObjectOptions, "deleteKmipObjectOptions")
	if err != nil {
		err = core.SDKErrorf(err, "", "struct-validation-error", common.GetComponentInfo())
		return
	}

	pathParamsMap := map[string]string{
		"adapter_id": *deleteKmipObjectOptions.AdapterID,
		"id":         *deleteKmipObjectOptions.ID,
	}

	builder := core.NewRequestBuilder(core.DELETE)
	builder = builder.WithContext(ctx)
	builder.EnableGzipCompression = ibmKeyProtectApi.GetEnableGzipCompression()
	_, err = builder.ResolveRequestURL(ibmKeyProtectApi.Service.Options.URL, `/api/v2/kmip_adapters/{adapter_id}/kmip_objects/{id}`, pathParamsMap)
	if err != nil {
		err = core.SDKErrorf(err, "", "url-resolve-error", common.GetComponentInfo())
		return
	}

	sdkHeaders := common.GetSdkHeaders("ibm_key_protect_api", "V2", "DeleteKmipObject")
	for headerName, headerValue := range sdkHeaders {
		builder.AddHeader(headerName, headerValue)
	}

	for headerName, headerValue := range deleteKmipObjectOptions.Headers {
		builder.AddHeader(headerName, headerValue)
	}
	if deleteKmipObjectOptions.BluemixInstance != nil {
		builder.AddHeader("Bluemix-Instance", fmt.Sprint(*deleteKmipObjectOptions.BluemixInstance))
	}
	if deleteKmipObjectOptions.CorrelationID != nil {
		builder.AddHeader("Correlation-Id", fmt.Sprint(*deleteKmipObjectOptions.CorrelationID))
	}

	if deleteKmipObjectOptions.Force != nil {
		builder.AddQuery("force", fmt.Sprint(*deleteKmipObjectOptions.Force))
	}

	request, err := builder.Build()
	if err != nil {
		err = core.SDKErrorf(err, "", "build-error", common.GetComponentInfo())
		return
	}

	response, err = ibmKeyProtectApi.Service.Request(request, nil)
	if err != nil {
		core.EnrichHTTPProblem(err, "delete_kmip_object", getServiceComponentInfo())
		err = core.SDKErrorf(err, "", "http-request-err", common.GetComponentInfo())
		return
	}

	return
}

// GetKmipClientCertificates : List client certificates of a KMIP Adapter
// List client certificates of a KMIP Adapter.
func (ibmKeyProtectApi *IbmKeyProtectApiV2) GetKmipClientCertificates(getKmipClientCertificatesOptions *GetKmipClientCertificatesOptions) (result *ListKMIPPartialClientCertificatesWithTotalCount, response *core.DetailedResponse, err error) {
	result, response, err = ibmKeyProtectApi.GetKmipClientCertificatesWithContext(context.Background(), getKmipClientCertificatesOptions)
	err = core.RepurposeSDKProblem(err, "")
	return
}

// GetKmipClientCertificatesWithContext is an alternate form of the GetKmipClientCertificates method which supports a Context parameter
func (ibmKeyProtectApi *IbmKeyProtectApiV2) GetKmipClientCertificatesWithContext(ctx context.Context, getKmipClientCertificatesOptions *GetKmipClientCertificatesOptions) (result *ListKMIPPartialClientCertificatesWithTotalCount, response *core.DetailedResponse, err error) {
	err = core.ValidateNotNil(getKmipClientCertificatesOptions, "getKmipClientCertificatesOptions cannot be nil")
	if err != nil {
		err = core.SDKErrorf(err, "", "unexpected-nil-param", common.GetComponentInfo())
		return
	}
	err = core.ValidateStruct(getKmipClientCertificatesOptions, "getKmipClientCertificatesOptions")
	if err != nil {
		err = core.SDKErrorf(err, "", "struct-validation-error", common.GetComponentInfo())
		return
	}

	pathParamsMap := map[string]string{
		"adapter_id": *getKmipClientCertificatesOptions.AdapterID,
	}

	builder := core.NewRequestBuilder(core.GET)
	builder = builder.WithContext(ctx)
	builder.EnableGzipCompression = ibmKeyProtectApi.GetEnableGzipCompression()
	_, err = builder.ResolveRequestURL(ibmKeyProtectApi.Service.Options.URL, `/api/v2/kmip_adapters/{adapter_id}/certificates`, pathParamsMap)
	if err != nil {
		err = core.SDKErrorf(err, "", "url-resolve-error", common.GetComponentInfo())
		return
	}

	sdkHeaders := common.GetSdkHeaders("ibm_key_protect_api", "V2", "GetKmipClientCertificates")
	for headerName, headerValue := range sdkHeaders {
		builder.AddHeader(headerName, headerValue)
	}

	for headerName, headerValue := range getKmipClientCertificatesOptions.Headers {
		builder.AddHeader(headerName, headerValue)
	}
	builder.AddHeader("Accept", "application/json")
	if getKmipClientCertificatesOptions.BluemixInstance != nil {
		builder.AddHeader("Bluemix-Instance", fmt.Sprint(*getKmipClientCertificatesOptions.BluemixInstance))
	}
	if getKmipClientCertificatesOptions.CorrelationID != nil {
		builder.AddHeader("Correlation-Id", fmt.Sprint(*getKmipClientCertificatesOptions.CorrelationID))
	}

	if getKmipClientCertificatesOptions.Limit != nil {
		builder.AddQuery("limit", fmt.Sprint(*getKmipClientCertificatesOptions.Limit))
	}
	if getKmipClientCertificatesOptions.Offset != nil {
		builder.AddQuery("offset", fmt.Sprint(*getKmipClientCertificatesOptions.Offset))
	}
	if getKmipClientCertificatesOptions.TotalCount != nil {
		builder.AddQuery("totalCount", fmt.Sprint(*getKmipClientCertificatesOptions.TotalCount))
	}

	request, err := builder.Build()
	if err != nil {
		err = core.SDKErrorf(err, "", "build-error", common.GetComponentInfo())
		return
	}

	var rawResponse map[string]json.RawMessage
	response, err = ibmKeyProtectApi.Service.Request(request, &rawResponse)
	if err != nil {
		core.EnrichHTTPProblem(err, "get_kmip_client_certificates", getServiceComponentInfo())
		err = core.SDKErrorf(err, "", "http-request-err", common.GetComponentInfo())
		return
	}
	if rawResponse != nil {
		err = core.UnmarshalModel(rawResponse, "", &result, UnmarshalListKMIPPartialClientCertificatesWithTotalCount)
		if err != nil {
			err = core.SDKErrorf(err, "", "unmarshal-resp-error", common.GetComponentInfo())
			return
		}
		response.Result = result
	}

	return
}

// AddKmipClientCertificate : Add a client certificate to a KMIP Adapter
// Add a client certificate to a KMIP Adapter. It might take up to 5 minutes for a KMIP call using the newly add
// certificate to pass authentication. A maximum of 200 client certificates can be associated with a KMIP Adapter at a
// time.
func (ibmKeyProtectApi *IbmKeyProtectApiV2) AddKmipClientCertificate(addKmipClientCertificateOptions *AddKmipClientCertificateOptions) (result *ListKMIPClientCertificates, response *core.DetailedResponse, err error) {
	result, response, err = ibmKeyProtectApi.AddKmipClientCertificateWithContext(context.Background(), addKmipClientCertificateOptions)
	err = core.RepurposeSDKProblem(err, "")
	return
}

// AddKmipClientCertificateWithContext is an alternate form of the AddKmipClientCertificate method which supports a Context parameter
func (ibmKeyProtectApi *IbmKeyProtectApiV2) AddKmipClientCertificateWithContext(ctx context.Context, addKmipClientCertificateOptions *AddKmipClientCertificateOptions) (result *ListKMIPClientCertificates, response *core.DetailedResponse, err error) {
	err = core.ValidateNotNil(addKmipClientCertificateOptions, "addKmipClientCertificateOptions cannot be nil")
	if err != nil {
		err = core.SDKErrorf(err, "", "unexpected-nil-param", common.GetComponentInfo())
		return
	}
	err = core.ValidateStruct(addKmipClientCertificateOptions, "addKmipClientCertificateOptions")
	if err != nil {
		err = core.SDKErrorf(err, "", "struct-validation-error", common.GetComponentInfo())
		return
	}

	pathParamsMap := map[string]string{
		"adapter_id": *addKmipClientCertificateOptions.AdapterID,
	}

	builder := core.NewRequestBuilder(core.POST)
	builder = builder.WithContext(ctx)
	builder.EnableGzipCompression = ibmKeyProtectApi.GetEnableGzipCompression()
	_, err = builder.ResolveRequestURL(ibmKeyProtectApi.Service.Options.URL, `/api/v2/kmip_adapters/{adapter_id}/certificates`, pathParamsMap)
	if err != nil {
		err = core.SDKErrorf(err, "", "url-resolve-error", common.GetComponentInfo())
		return
	}

	sdkHeaders := common.GetSdkHeaders("ibm_key_protect_api", "V2", "AddKmipClientCertificate")
	for headerName, headerValue := range sdkHeaders {
		builder.AddHeader(headerName, headerValue)
	}

	for headerName, headerValue := range addKmipClientCertificateOptions.Headers {
		builder.AddHeader(headerName, headerValue)
	}
	builder.AddHeader("Accept", "application/json")
	builder.AddHeader("Content-Type", "application/json")
	if addKmipClientCertificateOptions.BluemixInstance != nil {
		builder.AddHeader("Bluemix-Instance", fmt.Sprint(*addKmipClientCertificateOptions.BluemixInstance))
	}
	if addKmipClientCertificateOptions.CorrelationID != nil {
		builder.AddHeader("Correlation-Id", fmt.Sprint(*addKmipClientCertificateOptions.CorrelationID))
	}

	body := make(map[string]interface{})
	if addKmipClientCertificateOptions.Metadata != nil {
		body["metadata"] = addKmipClientCertificateOptions.Metadata
	}
	if addKmipClientCertificateOptions.Resources != nil {
		body["resources"] = addKmipClientCertificateOptions.Resources
	}
	_, err = builder.SetBodyContentJSON(body)
	if err != nil {
		err = core.SDKErrorf(err, "", "set-json-body-error", common.GetComponentInfo())
		return
	}

	request, err := builder.Build()
	if err != nil {
		err = core.SDKErrorf(err, "", "build-error", common.GetComponentInfo())
		return
	}

	var rawResponse map[string]json.RawMessage
	response, err = ibmKeyProtectApi.Service.Request(request, &rawResponse)
	if err != nil {
		core.EnrichHTTPProblem(err, "add_kmip_client_certificate", getServiceComponentInfo())
		err = core.SDKErrorf(err, "", "http-request-err", common.GetComponentInfo())
		return
	}
	if rawResponse != nil {
		err = core.UnmarshalModel(rawResponse, "", &result, UnmarshalListKMIPClientCertificates)
		if err != nil {
			err = core.SDKErrorf(err, "", "unmarshal-resp-error", common.GetComponentInfo())
			return
		}
		response.Result = result
	}

	return
}

// GetKmipClientCertificate : Retrieve a client certificate from a KMIP Adapter
// Retrieves a client certificate from a KMIP Adapter using its id / name.
func (ibmKeyProtectApi *IbmKeyProtectApiV2) GetKmipClientCertificate(getKmipClientCertificateOptions *GetKmipClientCertificateOptions) (result *ListKMIPClientCertificates, response *core.DetailedResponse, err error) {
	result, response, err = ibmKeyProtectApi.GetKmipClientCertificateWithContext(context.Background(), getKmipClientCertificateOptions)
	err = core.RepurposeSDKProblem(err, "")
	return
}

// GetKmipClientCertificateWithContext is an alternate form of the GetKmipClientCertificate method which supports a Context parameter
func (ibmKeyProtectApi *IbmKeyProtectApiV2) GetKmipClientCertificateWithContext(ctx context.Context, getKmipClientCertificateOptions *GetKmipClientCertificateOptions) (result *ListKMIPClientCertificates, response *core.DetailedResponse, err error) {
	err = core.ValidateNotNil(getKmipClientCertificateOptions, "getKmipClientCertificateOptions cannot be nil")
	if err != nil {
		err = core.SDKErrorf(err, "", "unexpected-nil-param", common.GetComponentInfo())
		return
	}
	err = core.ValidateStruct(getKmipClientCertificateOptions, "getKmipClientCertificateOptions")
	if err != nil {
		err = core.SDKErrorf(err, "", "struct-validation-error", common.GetComponentInfo())
		return
	}

	pathParamsMap := map[string]string{
		"adapter_id": *getKmipClientCertificateOptions.AdapterID,
		"id":         *getKmipClientCertificateOptions.ID,
	}

	builder := core.NewRequestBuilder(core.GET)
	builder = builder.WithContext(ctx)
	builder.EnableGzipCompression = ibmKeyProtectApi.GetEnableGzipCompression()
	_, err = builder.ResolveRequestURL(ibmKeyProtectApi.Service.Options.URL, `/api/v2/kmip_adapters/{adapter_id}/certificates/{id}`, pathParamsMap)
	if err != nil {
		err = core.SDKErrorf(err, "", "url-resolve-error", common.GetComponentInfo())
		return
	}

	sdkHeaders := common.GetSdkHeaders("ibm_key_protect_api", "V2", "GetKmipClientCertificate")
	for headerName, headerValue := range sdkHeaders {
		builder.AddHeader(headerName, headerValue)
	}

	for headerName, headerValue := range getKmipClientCertificateOptions.Headers {
		builder.AddHeader(headerName, headerValue)
	}
	builder.AddHeader("Accept", "application/json")
	if getKmipClientCertificateOptions.BluemixInstance != nil {
		builder.AddHeader("Bluemix-Instance", fmt.Sprint(*getKmipClientCertificateOptions.BluemixInstance))
	}
	if getKmipClientCertificateOptions.CorrelationID != nil {
		builder.AddHeader("Correlation-Id", fmt.Sprint(*getKmipClientCertificateOptions.CorrelationID))
	}

	request, err := builder.Build()
	if err != nil {
		err = core.SDKErrorf(err, "", "build-error", common.GetComponentInfo())
		return
	}

	var rawResponse map[string]json.RawMessage
	response, err = ibmKeyProtectApi.Service.Request(request, &rawResponse)
	if err != nil {
		core.EnrichHTTPProblem(err, "get_kmip_client_certificate", getServiceComponentInfo())
		err = core.SDKErrorf(err, "", "http-request-err", common.GetComponentInfo())
		return
	}
	if rawResponse != nil {
		err = core.UnmarshalModel(rawResponse, "", &result, UnmarshalListKMIPClientCertificates)
		if err != nil {
			err = core.SDKErrorf(err, "", "unmarshal-resp-error", common.GetComponentInfo())
			return
		}
		response.Result = result
	}

	return
}

// DeleteKmipClientCertificate : Delete a client certificate from a KMIP Adapter
// Removes a client certificate from a KMIP Adapter given its id / name. It might take up to 5 minutes for a KMIP call
// using deleted certificate to fail authentication.
func (ibmKeyProtectApi *IbmKeyProtectApiV2) DeleteKmipClientCertificate(deleteKmipClientCertificateOptions *DeleteKmipClientCertificateOptions) (response *core.DetailedResponse, err error) {
	response, err = ibmKeyProtectApi.DeleteKmipClientCertificateWithContext(context.Background(), deleteKmipClientCertificateOptions)
	err = core.RepurposeSDKProblem(err, "")
	return
}

// DeleteKmipClientCertificateWithContext is an alternate form of the DeleteKmipClientCertificate method which supports a Context parameter
func (ibmKeyProtectApi *IbmKeyProtectApiV2) DeleteKmipClientCertificateWithContext(ctx context.Context, deleteKmipClientCertificateOptions *DeleteKmipClientCertificateOptions) (response *core.DetailedResponse, err error) {
	err = core.ValidateNotNil(deleteKmipClientCertificateOptions, "deleteKmipClientCertificateOptions cannot be nil")
	if err != nil {
		err = core.SDKErrorf(err, "", "unexpected-nil-param", common.GetComponentInfo())
		return
	}
	err = core.ValidateStruct(deleteKmipClientCertificateOptions, "deleteKmipClientCertificateOptions")
	if err != nil {
		err = core.SDKErrorf(err, "", "struct-validation-error", common.GetComponentInfo())
		return
	}

	pathParamsMap := map[string]string{
		"adapter_id": *deleteKmipClientCertificateOptions.AdapterID,
		"id":         *deleteKmipClientCertificateOptions.ID,
	}

	builder := core.NewRequestBuilder(core.DELETE)
	builder = builder.WithContext(ctx)
	builder.EnableGzipCompression = ibmKeyProtectApi.GetEnableGzipCompression()
	_, err = builder.ResolveRequestURL(ibmKeyProtectApi.Service.Options.URL, `/api/v2/kmip_adapters/{adapter_id}/certificates/{id}`, pathParamsMap)
	if err != nil {
		err = core.SDKErrorf(err, "", "url-resolve-error", common.GetComponentInfo())
		return
	}

	sdkHeaders := common.GetSdkHeaders("ibm_key_protect_api", "V2", "DeleteKmipClientCertificate")
	for headerName, headerValue := range sdkHeaders {
		builder.AddHeader(headerName, headerValue)
	}

	for headerName, headerValue := range deleteKmipClientCertificateOptions.Headers {
		builder.AddHeader(headerName, headerValue)
	}
	if deleteKmipClientCertificateOptions.BluemixInstance != nil {
		builder.AddHeader("Bluemix-Instance", fmt.Sprint(*deleteKmipClientCertificateOptions.BluemixInstance))
	}
	if deleteKmipClientCertificateOptions.CorrelationID != nil {
		builder.AddHeader("Correlation-Id", fmt.Sprint(*deleteKmipClientCertificateOptions.CorrelationID))
	}

	request, err := builder.Build()
	if err != nil {
		err = core.SDKErrorf(err, "", "build-error", common.GetComponentInfo())
		return
	}

	response, err = ibmKeyProtectApi.Service.Request(request, nil)
	if err != nil {
		core.EnrichHTTPProblem(err, "delete_kmip_client_certificate", getServiceComponentInfo())
		err = core.SDKErrorf(err, "", "http-request-err", common.GetComponentInfo())
		return
	}

	return
}
func getServiceComponentInfo() *core.ProblemComponent {
	return core.NewProblemComponent(DefaultServiceName, "2.0.0")
}

// ActionOnKeyOptions : The ActionOnKey options.
type ActionOnKeyOptions struct {
	// The v4 UUID that uniquely identifies the key.
	ID *string `json:"id" validate:"required,ne="`

	// The IBM Cloud instance ID that identifies your Key Protect service instance.
	BluemixInstance *string `json:"Bluemix-Instance" validate:"required"`

	// The action to perform on the specified key.
	Action *string `json:"action" validate:"required"`

	// The base request for key actions.
	KeyActionBody io.ReadCloser `json:"KeyActionBody" validate:"required"`

	// The v4 UUID used to correlate and track transactions.
	CorrelationID *string `json:"Correlation-Id,omitempty"`

	// The ID of the key ring that the specified key is a part of. When the header is not specified, Key Protect will
	// perform a key ring lookup. For a more optimized request, specify the key ring on every call. The key ring ID of keys
	// that are created without an `X-Kms-Key-Ring` header is: `default`.
	XKmsKeyRing *string `json:"X-Kms-Key-Ring,omitempty"`

	// Alters server behavior for POST or DELETE operations. A header with `return=minimal` causes the service to return
	// only the key identifier as metadata. A header containing `return=representation` returns both the key material and
	// metadata in the response entity-body. If the key has been designated as a root key, the system cannot return the key
	// material.
	// **Note:** During POST operations, Key Protect may not immediately return the key material due to key generation
	// time. To retrieve the key material, you can perform a subsequent `GET /keys/{id}` request.
	Prefer *string `json:"Prefer,omitempty"`

	// Allows users to set headers on API requests.
	Headers map[string]string
}

// Constants associated with the ActionOnKeyOptions.Action property.
// The action to perform on the specified key.
const (
	ActionOnKeyOptions_Action_Disable             = "disable"
	ActionOnKeyOptions_Action_Enable              = "enable"
	ActionOnKeyOptions_Action_Restore             = "restore"
	ActionOnKeyOptions_Action_Rewrap              = "rewrap"
	ActionOnKeyOptions_Action_Rotate              = "rotate"
	ActionOnKeyOptions_Action_Setkeyfordeletion   = "setKeyForDeletion"
	ActionOnKeyOptions_Action_Unsetkeyfordeletion = "unsetKeyForDeletion"
	ActionOnKeyOptions_Action_Unwrap              = "unwrap"
	ActionOnKeyOptions_Action_Wrap                = "wrap"
)

// Constants associated with the ActionOnKeyOptions.Prefer property.
// Alters server behavior for POST or DELETE operations. A header with `return=minimal` causes the service to return
// only the key identifier as metadata. A header containing `return=representation` returns both the key material and
// metadata in the response entity-body. If the key has been designated as a root key, the system cannot return the key
// material.
// **Note:** During POST operations, Key Protect may not immediately return the key material due to key generation time.
// To retrieve the key material, you can perform a subsequent `GET /keys/{id}` request.
const (
	ActionOnKeyOptions_Prefer_ReturnMinimal        = "return=minimal"
	ActionOnKeyOptions_Prefer_ReturnRepresentation = "return=representation"
)

// NewActionOnKeyOptions : Instantiate ActionOnKeyOptions
func (*IbmKeyProtectApiV2) NewActionOnKeyOptions(id string, bluemixInstance string, action string, keyActionBody io.ReadCloser) *ActionOnKeyOptions {
	return &ActionOnKeyOptions{
		ID:              core.StringPtr(id),
		BluemixInstance: core.StringPtr(bluemixInstance),
		Action:          core.StringPtr(action),
		KeyActionBody:   keyActionBody,
	}
}

// SetID : Allow user to set ID
func (_options *ActionOnKeyOptions) SetID(id string) *ActionOnKeyOptions {
	_options.ID = core.StringPtr(id)
	return _options
}

// SetBluemixInstance : Allow user to set BluemixInstance
func (_options *ActionOnKeyOptions) SetBluemixInstance(bluemixInstance string) *ActionOnKeyOptions {
	_options.BluemixInstance = core.StringPtr(bluemixInstance)
	return _options
}

// SetAction : Allow user to set Action
func (_options *ActionOnKeyOptions) SetAction(action string) *ActionOnKeyOptions {
	_options.Action = core.StringPtr(action)
	return _options
}

// SetKeyActionBody : Allow user to set KeyActionBody
func (_options *ActionOnKeyOptions) SetKeyActionBody(keyActionBody io.ReadCloser) *ActionOnKeyOptions {
	_options.KeyActionBody = keyActionBody
	return _options
}

// SetCorrelationID : Allow user to set CorrelationID
func (_options *ActionOnKeyOptions) SetCorrelationID(correlationID string) *ActionOnKeyOptions {
	_options.CorrelationID = core.StringPtr(correlationID)
	return _options
}

// SetXKmsKeyRing : Allow user to set XKmsKeyRing
func (_options *ActionOnKeyOptions) SetXKmsKeyRing(xKmsKeyRing string) *ActionOnKeyOptions {
	_options.XKmsKeyRing = core.StringPtr(xKmsKeyRing)
	return _options
}

// SetPrefer : Allow user to set Prefer
func (_options *ActionOnKeyOptions) SetPrefer(prefer string) *ActionOnKeyOptions {
	_options.Prefer = core.StringPtr(prefer)
	return _options
}

// SetHeaders : Allow user to set Headers
func (options *ActionOnKeyOptions) SetHeaders(param map[string]string) *ActionOnKeyOptions {
	options.Headers = param
	return options
}

// AddKmipClientCertificateOptions : The AddKmipClientCertificate options.
type AddKmipClientCertificateOptions struct {
	// The name or v4 UUID of the KMIP Adapter that uniquely identifies it.
	AdapterID *string `json:"adapter_id" validate:"required,ne="`

	// The IBM Cloud instance ID that identifies your Key Protect service instance.
	BluemixInstance *string `json:"Bluemix-Instance" validate:"required"`

	// The metadata that describes the resource array.
	Metadata *CollectionMetadata `json:"metadata" validate:"required"`

	// A collection of resources.
	Resources []CreateKMIPClientCertificateObject `json:"resources" validate:"required"`

	// The v4 UUID used to correlate and track transactions.
	CorrelationID *string `json:"Correlation-Id,omitempty"`

	// Allows users to set headers on API requests.
	Headers map[string]string
}

// NewAddKmipClientCertificateOptions : Instantiate AddKmipClientCertificateOptions
func (*IbmKeyProtectApiV2) NewAddKmipClientCertificateOptions(adapterID string, bluemixInstance string, metadata *CollectionMetadata, resources []CreateKMIPClientCertificateObject) *AddKmipClientCertificateOptions {
	return &AddKmipClientCertificateOptions{
		AdapterID:       core.StringPtr(adapterID),
		BluemixInstance: core.StringPtr(bluemixInstance),
		Metadata:        metadata,
		Resources:       resources,
	}
}

// SetAdapterID : Allow user to set AdapterID
func (_options *AddKmipClientCertificateOptions) SetAdapterID(adapterID string) *AddKmipClientCertificateOptions {
	_options.AdapterID = core.StringPtr(adapterID)
	return _options
}

// SetBluemixInstance : Allow user to set BluemixInstance
func (_options *AddKmipClientCertificateOptions) SetBluemixInstance(bluemixInstance string) *AddKmipClientCertificateOptions {
	_options.BluemixInstance = core.StringPtr(bluemixInstance)
	return _options
}

// SetMetadata : Allow user to set Metadata
func (_options *AddKmipClientCertificateOptions) SetMetadata(metadata *CollectionMetadata) *AddKmipClientCertificateOptions {
	_options.Metadata = metadata
	return _options
}

// SetResources : Allow user to set Resources
func (_options *AddKmipClientCertificateOptions) SetResources(resources []CreateKMIPClientCertificateObject) *AddKmipClientCertificateOptions {
	_options.Resources = resources
	return _options
}

// SetCorrelationID : Allow user to set CorrelationID
func (_options *AddKmipClientCertificateOptions) SetCorrelationID(correlationID string) *AddKmipClientCertificateOptions {
	_options.CorrelationID = core.StringPtr(correlationID)
	return _options
}

// SetHeaders : Allow user to set Headers
func (options *AddKmipClientCertificateOptions) SetHeaders(param map[string]string) *AddKmipClientCertificateOptions {
	options.Headers = param
	return options
}

// AllowedIPPort : Properties associated with the port associated with an instance with an allowed IP policy.
type AllowedIPPort struct {
	// The metadata that describes the resource array.
	Metadata *CollectionMetadata `json:"metadata,omitempty"`

	// A collection of resources.
	Resources []AllowedIPPortResource `json:"resources,omitempty"`
}

// UnmarshalAllowedIPPort unmarshals an instance of AllowedIPPort from the specified map of raw messages.
func UnmarshalAllowedIPPort(m map[string]json.RawMessage, result interface{}) (err error) {
	obj := new(AllowedIPPort)
	err = core.UnmarshalModel(m, "metadata", &obj.Metadata, UnmarshalCollectionMetadata)
	if err != nil {
		err = core.SDKErrorf(err, "", "metadata-error", common.GetComponentInfo())
		return
	}
	err = core.UnmarshalModel(m, "resources", &obj.Resources, UnmarshalAllowedIPPortResource)
	if err != nil {
		err = core.SDKErrorf(err, "", "resources-error", common.GetComponentInfo())
		return
	}
	reflect.ValueOf(result).Elem().Set(reflect.ValueOf(obj))
	return
}

// AllowedIPPortResource : Metadata of the port associated with an instance with an allowed IP policy.
type AllowedIPPortResource struct {
	// The port required to access an instance with an allowed IP policy via the Key Protect private service endpoint.
	// Cannot be used with the Key Protect public service endpoint. For more information, see [accessing an instance via
	// private endpoint](/docs/key-protect?topic=key-protect-manage-allowed-ip#access-allowed-ip-private-endpoint) for
	// instructions on how to use the `private_endpoint_port` value.
	PrivateEndpointPort *int64 `json:"private_endpoint_port,omitempty"`
}

// UnmarshalAllowedIPPortResource unmarshals an instance of AllowedIPPortResource from the specified map of raw messages.
func UnmarshalAllowedIPPortResource(m map[string]json.RawMessage, result interface{}) (err error) {
	obj := new(AllowedIPPortResource)
	err = core.UnmarshalPrimitive(m, "private_endpoint_port", &obj.PrivateEndpointPort)
	if err != nil {
		err = core.SDKErrorf(err, "", "private_endpoint_port-error", common.GetComponentInfo())
		return
	}
	reflect.ValueOf(result).Elem().Set(reflect.ValueOf(obj))
	return
}

// CollectionMetadata : The metadata that describes the resource array.
type CollectionMetadata struct {
	// The type of resources in the resource array.
	CollectionType *string `json:"collectionType" validate:"required"`

	// The number of elements in the resource array.
	CollectionTotal *int64 `json:"collectionTotal" validate:"required"`
}

// Constants associated with the CollectionMetadata.CollectionType property.
// The type of resources in the resource array.
const (
	CollectionMetadata_CollectionType_ApplicationVndIbmKmsAliasJSON                 = "application/vnd.ibm.kms.alias+json"
	CollectionMetadata_CollectionType_ApplicationVndIbmKmsAllowedIpMetadataJSON     = "application/vnd.ibm.kms.allowed_ip_metadata+json"
	CollectionMetadata_CollectionType_ApplicationVndIbmKmsCrnJSON                   = "application/vnd.ibm.kms.crn+json"
	CollectionMetadata_CollectionType_ApplicationVndIbmKmsErrorJSON                 = "application/vnd.ibm.kms.error+json"
	CollectionMetadata_CollectionType_ApplicationVndIbmKmsEventAcknowledgeJSON      = "application/vnd.ibm.kms.event_acknowledge+json"
	CollectionMetadata_CollectionType_ApplicationVndIbmKmsImportTokenJSON           = "application/vnd.ibm.kms.import_token+json"
	CollectionMetadata_CollectionType_ApplicationVndIbmKmsKeyActionJSON             = "application/vnd.ibm.kms.key_action+json"
	CollectionMetadata_CollectionType_ApplicationVndIbmKmsKeyJSON                   = "application/vnd.ibm.kms.key+json"
	CollectionMetadata_CollectionType_ApplicationVndIbmKmsKeyRingJSON               = "application/vnd.ibm.kms.key_ring+json"
	CollectionMetadata_CollectionType_ApplicationVndIbmKmsKmipAdapterJSON           = "application/vnd.ibm.kms.kmip_adapter+json"
	CollectionMetadata_CollectionType_ApplicationVndIbmKmsKmipClientCertificateJSON = "application/vnd.ibm.kms.kmip_client_certificate+json"
	CollectionMetadata_CollectionType_ApplicationVndIbmKmsKmipObjectJSON            = "application/vnd.ibm.kms.kmip_object+json"
	CollectionMetadata_CollectionType_ApplicationVndIbmKmsPolicyJSON                = "application/vnd.ibm.kms.policy+json"
	CollectionMetadata_CollectionType_ApplicationVndIbmKmsRegistrationInputJSON     = "application/vnd.ibm.kms.registration_input+json"
	CollectionMetadata_CollectionType_ApplicationVndIbmKmsRegistrationJSON          = "application/vnd.ibm.kms.registration+json"
	CollectionMetadata_CollectionType_ApplicationVndIbmKmsResourceCrnJSON           = "application/vnd.ibm.kms.resource_crn+json"
)

// NewCollectionMetadata : Instantiate CollectionMetadata (Generic Model Constructor)
func (*IbmKeyProtectApiV2) NewCollectionMetadata(collectionType string, collectionTotal int64) (_model *CollectionMetadata, err error) {
	_model = &CollectionMetadata{
		CollectionType:  core.StringPtr(collectionType),
		CollectionTotal: core.Int64Ptr(collectionTotal),
	}
	err = core.ValidateStruct(_model, "required parameters")
	if err != nil {
		err = core.SDKErrorf(err, "", "model-missing-required", common.GetComponentInfo())
	}
	return
}

// UnmarshalCollectionMetadata unmarshals an instance of CollectionMetadata from the specified map of raw messages.
func UnmarshalCollectionMetadata(m map[string]json.RawMessage, result interface{}) (err error) {
	obj := new(CollectionMetadata)
	err = core.UnmarshalPrimitive(m, "collectionType", &obj.CollectionType)
	if err != nil {
		err = core.SDKErrorf(err, "", "collectionType-error", common.GetComponentInfo())
		return
	}
	err = core.UnmarshalPrimitive(m, "collectionTotal", &obj.CollectionTotal)
	if err != nil {
		err = core.SDKErrorf(err, "", "collectionTotal-error", common.GetComponentInfo())
		return
	}
	reflect.ValueOf(result).Elem().Set(reflect.ValueOf(obj))
	return
}

// CollectionMetadataListKeys : The metadata that describes the list keys response.
type CollectionMetadataListKeys struct {
	// The type of resources in the resource array.
	CollectionType *string `json:"collectionType" validate:"required"`

	// The number of elements in the resource array.
	CollectionTotal *int64 `json:"collectionTotal" validate:"required"`

	// If present, indicates the search did not complete due to the searchable set of keys being too large. Please retry
	// your request with additional or more specific filters (i.e. extractable, state, etc.). To determine the size of the
	// searchable set of keys, please use `HEAD /api/v2/keys` with the desired search filters. For a search to be
	// performmed, the resulting set contain at most 5000 keys.
	IncompleteSearch *bool `json:"incompleteSearch,omitempty"`

	// Represents the parsed search query used for matching logic. Only returned when a search is requested.
	SearchQuery *ListKeysMetadataPropertiesSearchQuery `json:"searchQuery,omitempty"`
}

// Constants associated with the CollectionMetadataListKeys.CollectionType property.
// The type of resources in the resource array.
const (
	CollectionMetadataListKeys_CollectionType_ApplicationVndIbmKmsAliasJSON                 = "application/vnd.ibm.kms.alias+json"
	CollectionMetadataListKeys_CollectionType_ApplicationVndIbmKmsAllowedIpMetadataJSON     = "application/vnd.ibm.kms.allowed_ip_metadata+json"
	CollectionMetadataListKeys_CollectionType_ApplicationVndIbmKmsCrnJSON                   = "application/vnd.ibm.kms.crn+json"
	CollectionMetadataListKeys_CollectionType_ApplicationVndIbmKmsErrorJSON                 = "application/vnd.ibm.kms.error+json"
	CollectionMetadataListKeys_CollectionType_ApplicationVndIbmKmsEventAcknowledgeJSON      = "application/vnd.ibm.kms.event_acknowledge+json"
	CollectionMetadataListKeys_CollectionType_ApplicationVndIbmKmsImportTokenJSON           = "application/vnd.ibm.kms.import_token+json"
	CollectionMetadataListKeys_CollectionType_ApplicationVndIbmKmsKeyActionJSON             = "application/vnd.ibm.kms.key_action+json"
	CollectionMetadataListKeys_CollectionType_ApplicationVndIbmKmsKeyJSON                   = "application/vnd.ibm.kms.key+json"
	CollectionMetadataListKeys_CollectionType_ApplicationVndIbmKmsKeyRingJSON               = "application/vnd.ibm.kms.key_ring+json"
	CollectionMetadataListKeys_CollectionType_ApplicationVndIbmKmsKmipAdapterJSON           = "application/vnd.ibm.kms.kmip_adapter+json"
	CollectionMetadataListKeys_CollectionType_ApplicationVndIbmKmsKmipClientCertificateJSON = "application/vnd.ibm.kms.kmip_client_certificate+json"
	CollectionMetadataListKeys_CollectionType_ApplicationVndIbmKmsKmipObjectJSON            = "application/vnd.ibm.kms.kmip_object+json"
	CollectionMetadataListKeys_CollectionType_ApplicationVndIbmKmsPolicyJSON                = "application/vnd.ibm.kms.policy+json"
	CollectionMetadataListKeys_CollectionType_ApplicationVndIbmKmsRegistrationInputJSON     = "application/vnd.ibm.kms.registration_input+json"
	CollectionMetadataListKeys_CollectionType_ApplicationVndIbmKmsRegistrationJSON          = "application/vnd.ibm.kms.registration+json"
	CollectionMetadataListKeys_CollectionType_ApplicationVndIbmKmsResourceCrnJSON           = "application/vnd.ibm.kms.resource_crn+json"
)

// UnmarshalCollectionMetadataListKeys unmarshals an instance of CollectionMetadataListKeys from the specified map of raw messages.
func UnmarshalCollectionMetadataListKeys(m map[string]json.RawMessage, result interface{}) (err error) {
	obj := new(CollectionMetadataListKeys)
	err = core.UnmarshalPrimitive(m, "collectionType", &obj.CollectionType)
	if err != nil {
		err = core.SDKErrorf(err, "", "collectionType-error", common.GetComponentInfo())
		return
	}
	err = core.UnmarshalPrimitive(m, "collectionTotal", &obj.CollectionTotal)
	if err != nil {
		err = core.SDKErrorf(err, "", "collectionTotal-error", common.GetComponentInfo())
		return
	}
	err = core.UnmarshalPrimitive(m, "incompleteSearch", &obj.IncompleteSearch)
	if err != nil {
		err = core.SDKErrorf(err, "", "incompleteSearch-error", common.GetComponentInfo())
		return
	}
	err = core.UnmarshalModel(m, "searchQuery", &obj.SearchQuery, UnmarshalListKeysMetadataPropertiesSearchQuery)
	if err != nil {
		err = core.SDKErrorf(err, "", "searchQuery-error", common.GetComponentInfo())
		return
	}
	reflect.ValueOf(result).Elem().Set(reflect.ValueOf(obj))
	return
}

// CollectionMetadataOneOf : CollectionMetadataOneOf struct
// Models which "extend" this model:
// - CollectionMetadataOneOfCollectionMetadata
type CollectionMetadataOneOf struct {
	// The type of resources in the resource array.
	CollectionType *string `json:"collectionType,omitempty"`

	// The number of elements in the resource array.
	CollectionTotal *int64 `json:"collectionTotal,omitempty"`
}

// Constants associated with the CollectionMetadataOneOf.CollectionType property.
// The type of resources in the resource array.
const (
	CollectionMetadataOneOf_CollectionType_ApplicationVndIbmKmsAliasJSON                 = "application/vnd.ibm.kms.alias+json"
	CollectionMetadataOneOf_CollectionType_ApplicationVndIbmKmsAllowedIpMetadataJSON     = "application/vnd.ibm.kms.allowed_ip_metadata+json"
	CollectionMetadataOneOf_CollectionType_ApplicationVndIbmKmsCrnJSON                   = "application/vnd.ibm.kms.crn+json"
	CollectionMetadataOneOf_CollectionType_ApplicationVndIbmKmsErrorJSON                 = "application/vnd.ibm.kms.error+json"
	CollectionMetadataOneOf_CollectionType_ApplicationVndIbmKmsEventAcknowledgeJSON      = "application/vnd.ibm.kms.event_acknowledge+json"
	CollectionMetadataOneOf_CollectionType_ApplicationVndIbmKmsImportTokenJSON           = "application/vnd.ibm.kms.import_token+json"
	CollectionMetadataOneOf_CollectionType_ApplicationVndIbmKmsKeyActionJSON             = "application/vnd.ibm.kms.key_action+json"
	CollectionMetadataOneOf_CollectionType_ApplicationVndIbmKmsKeyJSON                   = "application/vnd.ibm.kms.key+json"
	CollectionMetadataOneOf_CollectionType_ApplicationVndIbmKmsKeyRingJSON               = "application/vnd.ibm.kms.key_ring+json"
	CollectionMetadataOneOf_CollectionType_ApplicationVndIbmKmsKmipAdapterJSON           = "application/vnd.ibm.kms.kmip_adapter+json"
	CollectionMetadataOneOf_CollectionType_ApplicationVndIbmKmsKmipClientCertificateJSON = "application/vnd.ibm.kms.kmip_client_certificate+json"
	CollectionMetadataOneOf_CollectionType_ApplicationVndIbmKmsKmipObjectJSON            = "application/vnd.ibm.kms.kmip_object+json"
	CollectionMetadataOneOf_CollectionType_ApplicationVndIbmKmsPolicyJSON                = "application/vnd.ibm.kms.policy+json"
	CollectionMetadataOneOf_CollectionType_ApplicationVndIbmKmsRegistrationInputJSON     = "application/vnd.ibm.kms.registration_input+json"
	CollectionMetadataOneOf_CollectionType_ApplicationVndIbmKmsRegistrationJSON          = "application/vnd.ibm.kms.registration+json"
	CollectionMetadataOneOf_CollectionType_ApplicationVndIbmKmsResourceCrnJSON           = "application/vnd.ibm.kms.resource_crn+json"
)

func (*CollectionMetadataOneOf) isaCollectionMetadataOneOf() bool {
	return true
}

type CollectionMetadataOneOfIntf interface {
	isaCollectionMetadataOneOf() bool
}

// UnmarshalCollectionMetadataOneOf unmarshals an instance of CollectionMetadataOneOf from the specified map of raw messages.
func UnmarshalCollectionMetadataOneOf(m map[string]json.RawMessage, result interface{}) (err error) {
	obj := new(CollectionMetadataOneOf)
	err = core.UnmarshalPrimitive(m, "collectionType", &obj.CollectionType)
	if err != nil {
		err = core.SDKErrorf(err, "", "collectionType-error", common.GetComponentInfo())
		return
	}
	err = core.UnmarshalPrimitive(m, "collectionTotal", &obj.CollectionTotal)
	if err != nil {
		err = core.SDKErrorf(err, "", "collectionTotal-error", common.GetComponentInfo())
		return
	}
	reflect.ValueOf(result).Elem().Set(reflect.ValueOf(obj))
	return
}

// CollectionMetadataWithTotalCount : The metadata that describes the resource array.
type CollectionMetadataWithTotalCount struct {
	// The type of resources in the resource array.
	CollectionType *string `json:"collectionType" validate:"required"`

	// The number of elements in the resource array.
	CollectionTotal *int64 `json:"collectionTotal" validate:"required"`

	// The total number of elements that match the request, disregarding limit and offset.
	TotalCount *int64 `json:"totalCount,omitempty"`
}

// Constants associated with the CollectionMetadataWithTotalCount.CollectionType property.
// The type of resources in the resource array.
const (
	CollectionMetadataWithTotalCount_CollectionType_ApplicationVndIbmKmsAliasJSON                 = "application/vnd.ibm.kms.alias+json"
	CollectionMetadataWithTotalCount_CollectionType_ApplicationVndIbmKmsAllowedIpMetadataJSON     = "application/vnd.ibm.kms.allowed_ip_metadata+json"
	CollectionMetadataWithTotalCount_CollectionType_ApplicationVndIbmKmsCrnJSON                   = "application/vnd.ibm.kms.crn+json"
	CollectionMetadataWithTotalCount_CollectionType_ApplicationVndIbmKmsErrorJSON                 = "application/vnd.ibm.kms.error+json"
	CollectionMetadataWithTotalCount_CollectionType_ApplicationVndIbmKmsEventAcknowledgeJSON      = "application/vnd.ibm.kms.event_acknowledge+json"
	CollectionMetadataWithTotalCount_CollectionType_ApplicationVndIbmKmsImportTokenJSON           = "application/vnd.ibm.kms.import_token+json"
	CollectionMetadataWithTotalCount_CollectionType_ApplicationVndIbmKmsKeyActionJSON             = "application/vnd.ibm.kms.key_action+json"
	CollectionMetadataWithTotalCount_CollectionType_ApplicationVndIbmKmsKeyJSON                   = "application/vnd.ibm.kms.key+json"
	CollectionMetadataWithTotalCount_CollectionType_ApplicationVndIbmKmsKeyRingJSON               = "application/vnd.ibm.kms.key_ring+json"
	CollectionMetadataWithTotalCount_CollectionType_ApplicationVndIbmKmsKmipAdapterJSON           = "application/vnd.ibm.kms.kmip_adapter+json"
	CollectionMetadataWithTotalCount_CollectionType_ApplicationVndIbmKmsKmipClientCertificateJSON = "application/vnd.ibm.kms.kmip_client_certificate+json"
	CollectionMetadataWithTotalCount_CollectionType_ApplicationVndIbmKmsKmipObjectJSON            = "application/vnd.ibm.kms.kmip_object+json"
	CollectionMetadataWithTotalCount_CollectionType_ApplicationVndIbmKmsPolicyJSON                = "application/vnd.ibm.kms.policy+json"
	CollectionMetadataWithTotalCount_CollectionType_ApplicationVndIbmKmsRegistrationInputJSON     = "application/vnd.ibm.kms.registration_input+json"
	CollectionMetadataWithTotalCount_CollectionType_ApplicationVndIbmKmsRegistrationJSON          = "application/vnd.ibm.kms.registration+json"
	CollectionMetadataWithTotalCount_CollectionType_ApplicationVndIbmKmsResourceCrnJSON           = "application/vnd.ibm.kms.resource_crn+json"
)

// UnmarshalCollectionMetadataWithTotalCount unmarshals an instance of CollectionMetadataWithTotalCount from the specified map of raw messages.
func UnmarshalCollectionMetadataWithTotalCount(m map[string]json.RawMessage, result interface{}) (err error) {
	obj := new(CollectionMetadataWithTotalCount)
	err = core.UnmarshalPrimitive(m, "collectionType", &obj.CollectionType)
	if err != nil {
		err = core.SDKErrorf(err, "", "collectionType-error", common.GetComponentInfo())
		return
	}
	err = core.UnmarshalPrimitive(m, "collectionTotal", &obj.CollectionTotal)
	if err != nil {
		err = core.SDKErrorf(err, "", "collectionTotal-error", common.GetComponentInfo())
		return
	}
	err = core.UnmarshalPrimitive(m, "totalCount", &obj.TotalCount)
	if err != nil {
		err = core.SDKErrorf(err, "", "totalCount-error", common.GetComponentInfo())
		return
	}
	reflect.ValueOf(result).Elem().Set(reflect.ValueOf(obj))
	return
}

// CreateKMIPAdapterObject : CreateKMIPAdapterObject struct
type CreateKMIPAdapterObject struct {
	// A human-readable name of the KMIP adapter unique within the kms instance. If one is not specified, one will be
	// autogenerated of the format `kmip_adapter_<random_string>`. To protect your privacy do not use personal data, such
	// as your name or location, as a name for your KMIP adapter. The name must be alphanumeric and cannot contain spaces
	// or special characters other than `-` or `_`. The name cannot be a UUID.
	Name *string `json:"name,omitempty"`

	// The optional description of the KMIP adapter. The maximum length is 240 characters. To protect your privacy, do not
	// use personal data, such as your name or location, as a description for your KMIP adapter.
	Description *string `json:"description,omitempty"`

	// The profile of KMIP adapter to be created.
	Profile *string `json:"profile" validate:"required"`

	// The data specific to the KMIP Adapter profile. This is a required field for profile `native_1.0`.
	ProfileData KMIPProfileDataBodyIntf `json:"profile_data,omitempty"`
}

// Constants associated with the CreateKMIPAdapterObject.Profile property.
// The profile of KMIP adapter to be created.
const (
	CreateKMIPAdapterObject_Profile_Native10 = "native_1.0"
)

// NewCreateKMIPAdapterObject : Instantiate CreateKMIPAdapterObject (Generic Model Constructor)
func (*IbmKeyProtectApiV2) NewCreateKMIPAdapterObject(profile string) (_model *CreateKMIPAdapterObject, err error) {
	_model = &CreateKMIPAdapterObject{
		Profile: core.StringPtr(profile),
	}
	err = core.ValidateStruct(_model, "required parameters")
	if err != nil {
		err = core.SDKErrorf(err, "", "model-missing-required", common.GetComponentInfo())
	}
	return
}

// UnmarshalCreateKMIPAdapterObject unmarshals an instance of CreateKMIPAdapterObject from the specified map of raw messages.
func UnmarshalCreateKMIPAdapterObject(m map[string]json.RawMessage, result interface{}) (err error) {
	obj := new(CreateKMIPAdapterObject)
	err = core.UnmarshalPrimitive(m, "name", &obj.Name)
	if err != nil {
		err = core.SDKErrorf(err, "", "name-error", common.GetComponentInfo())
		return
	}
	err = core.UnmarshalPrimitive(m, "description", &obj.Description)
	if err != nil {
		err = core.SDKErrorf(err, "", "description-error", common.GetComponentInfo())
		return
	}
	err = core.UnmarshalPrimitive(m, "profile", &obj.Profile)
	if err != nil {
		err = core.SDKErrorf(err, "", "profile-error", common.GetComponentInfo())
		return
	}
	err = core.UnmarshalModel(m, "profile_data", &obj.ProfileData, UnmarshalKMIPProfileDataBody)
	if err != nil {
		err = core.SDKErrorf(err, "", "profile_data-error", common.GetComponentInfo())
		return
	}
	reflect.ValueOf(result).Elem().Set(reflect.ValueOf(obj))
	return
}

// CreateKMIPClientCertificateObject : CreateKMIPClientCertificateObject struct
type CreateKMIPClientCertificateObject struct {
	// The client certificate to be associated with the KMIP Adapter. It should explicitly have the BEGIN CERTIFICATE and
	// END CERTIFICATE tags.
	Certificate *string `json:"certificate" validate:"required"`

	// A human-readable name that uniquely identifies a certificate within the given adapter. If one is not specified, one
	// will be autogenerated of the format `kmip_cert_<random_string>`. To protect your privacy do not use personal data,
	// such as your name or location, as a name for your client certificate. The name must be alphanumeric and cannot
	// contain spaces or special characters other than `-` or `_`. The name cannot be a UUID.
	Name *string `json:"name,omitempty"`
}

// NewCreateKMIPClientCertificateObject : Instantiate CreateKMIPClientCertificateObject (Generic Model Constructor)
func (*IbmKeyProtectApiV2) NewCreateKMIPClientCertificateObject(certificate string) (_model *CreateKMIPClientCertificateObject, err error) {
	_model = &CreateKMIPClientCertificateObject{
		Certificate: core.StringPtr(certificate),
	}
	err = core.ValidateStruct(_model, "required parameters")
	if err != nil {
		err = core.SDKErrorf(err, "", "model-missing-required", common.GetComponentInfo())
	}
	return
}

// UnmarshalCreateKMIPClientCertificateObject unmarshals an instance of CreateKMIPClientCertificateObject from the specified map of raw messages.
func UnmarshalCreateKMIPClientCertificateObject(m map[string]json.RawMessage, result interface{}) (err error) {
	obj := new(CreateKMIPClientCertificateObject)
	err = core.UnmarshalPrimitive(m, "certificate", &obj.Certificate)
	if err != nil {
		err = core.SDKErrorf(err, "", "certificate-error", common.GetComponentInfo())
		return
	}
	err = core.UnmarshalPrimitive(m, "name", &obj.Name)
	if err != nil {
		err = core.SDKErrorf(err, "", "name-error", common.GetComponentInfo())
		return
	}
	reflect.ValueOf(result).Elem().Set(reflect.ValueOf(obj))
	return
}

// CreateKeyAliasOptions : The CreateKeyAlias options.
type CreateKeyAliasOptions struct {
	// The v4 UUID or alias that uniquely identifies the key.
	ID *string `json:"id" validate:"required,ne="`

	// A human-readable alias that uniquely identifies a key. Each alias is unique only within the given instance and is
	// not reserved across the Key Protect service. Each key can have up to five aliases. There is no limit to the number
	// of aliases per instance. The length of the alias can be between 2 - 90 characters, inclusive. An alias must be
	// alphanumeric and cannot contain spaces or special characters other than '-' or '_'. Also, the alias cannot be a
	// version 4 UUID and must not be a Key Protect reserved name: `allowed_ip`, `key`, `keys`, `metadata`, `policy`,
	// `policies`, `registration`, `registrations`, `ring`, `rings`, `rotate`, `wrap`, `unwrap`, `rewrap`, `version`,
	// `versions`.
	Alias *string `json:"alias" validate:"required,ne="`

	// The IBM Cloud instance ID that identifies your Key Protect service instance.
	BluemixInstance *string `json:"Bluemix-Instance" validate:"required"`

	// The v4 UUID used to correlate and track transactions.
	CorrelationID *string `json:"Correlation-Id,omitempty"`

	// The ID of the key ring that the specified key is a part of. When the header is not specified, Key Protect will
	// perform a key ring lookup. For a more optimized request, specify the key ring on every call. The key ring ID of keys
	// that are created without an `X-Kms-Key-Ring` header is: `default`.
	XKmsKeyRing *string `json:"X-Kms-Key-Ring,omitempty"`

	// Allows users to set headers on API requests.
	Headers map[string]string
}

// NewCreateKeyAliasOptions : Instantiate CreateKeyAliasOptions
func (*IbmKeyProtectApiV2) NewCreateKeyAliasOptions(id string, alias string, bluemixInstance string) *CreateKeyAliasOptions {
	return &CreateKeyAliasOptions{
		ID:              core.StringPtr(id),
		Alias:           core.StringPtr(alias),
		BluemixInstance: core.StringPtr(bluemixInstance),
	}
}

// SetID : Allow user to set ID
func (_options *CreateKeyAliasOptions) SetID(id string) *CreateKeyAliasOptions {
	_options.ID = core.StringPtr(id)
	return _options
}

// SetAlias : Allow user to set Alias
func (_options *CreateKeyAliasOptions) SetAlias(alias string) *CreateKeyAliasOptions {
	_options.Alias = core.StringPtr(alias)
	return _options
}

// SetBluemixInstance : Allow user to set BluemixInstance
func (_options *CreateKeyAliasOptions) SetBluemixInstance(bluemixInstance string) *CreateKeyAliasOptions {
	_options.BluemixInstance = core.StringPtr(bluemixInstance)
	return _options
}

// SetCorrelationID : Allow user to set CorrelationID
func (_options *CreateKeyAliasOptions) SetCorrelationID(correlationID string) *CreateKeyAliasOptions {
	_options.CorrelationID = core.StringPtr(correlationID)
	return _options
}

// SetXKmsKeyRing : Allow user to set XKmsKeyRing
func (_options *CreateKeyAliasOptions) SetXKmsKeyRing(xKmsKeyRing string) *CreateKeyAliasOptions {
	_options.XKmsKeyRing = core.StringPtr(xKmsKeyRing)
	return _options
}

// SetHeaders : Allow user to set Headers
func (options *CreateKeyAliasOptions) SetHeaders(param map[string]string) *CreateKeyAliasOptions {
	options.Headers = param
	return options
}

// CreateKeyOptions : The CreateKey options.
type CreateKeyOptions struct {
	// The IBM Cloud instance ID that identifies your Key Protect service instance.
	BluemixInstance *string `json:"Bluemix-Instance" validate:"required"`

	// The base request for creating a new key.
	KeyCreateBody io.ReadCloser `json:"KeyCreateBody" validate:"required"`

	// The v4 UUID used to correlate and track transactions.
	CorrelationID *string `json:"Correlation-Id,omitempty"`

	// Alters server behavior for POST or DELETE operations. A header with `return=minimal` causes the service to return
	// only the key identifier as metadata. A header containing `return=representation` returns both the key material and
	// metadata in the response entity-body. If the key has been designated as a root key, the system cannot return the key
	// material.
	// **Note:** During POST operations, Key Protect may not immediately return the key material due to key generation
	// time. To retrieve the key material, you can perform a subsequent `GET /keys/{id}` request.
	Prefer *string `json:"Prefer,omitempty"`

	// The ID of the key ring that the specified key belongs to. When the header is not specified, Key Protect will perform
	// a key ring lookup. For a more optimized request, specify the key ring on every call. The key ring ID of keys that
	// are created without an `X-Kms-Key-Ring` header is: `default`.
	XKmsKeyRing *string `json:"X-Kms-Key-Ring,omitempty"`

	// Allows users to set headers on API requests.
	Headers map[string]string
}

// Constants associated with the CreateKeyOptions.Prefer property.
// Alters server behavior for POST or DELETE operations. A header with `return=minimal` causes the service to return
// only the key identifier as metadata. A header containing `return=representation` returns both the key material and
// metadata in the response entity-body. If the key has been designated as a root key, the system cannot return the key
// material.
// **Note:** During POST operations, Key Protect may not immediately return the key material due to key generation time.
// To retrieve the key material, you can perform a subsequent `GET /keys/{id}` request.
const (
	CreateKeyOptions_Prefer_ReturnMinimal        = "return=minimal"
	CreateKeyOptions_Prefer_ReturnRepresentation = "return=representation"
)

// NewCreateKeyOptions : Instantiate CreateKeyOptions
func (*IbmKeyProtectApiV2) NewCreateKeyOptions(bluemixInstance string, keyCreateBody io.ReadCloser) *CreateKeyOptions {
	return &CreateKeyOptions{
		BluemixInstance: core.StringPtr(bluemixInstance),
		KeyCreateBody:   keyCreateBody,
	}
}

// SetBluemixInstance : Allow user to set BluemixInstance
func (_options *CreateKeyOptions) SetBluemixInstance(bluemixInstance string) *CreateKeyOptions {
	_options.BluemixInstance = core.StringPtr(bluemixInstance)
	return _options
}

// SetKeyCreateBody : Allow user to set KeyCreateBody
func (_options *CreateKeyOptions) SetKeyCreateBody(keyCreateBody io.ReadCloser) *CreateKeyOptions {
	_options.KeyCreateBody = keyCreateBody
	return _options
}

// SetCorrelationID : Allow user to set CorrelationID
func (_options *CreateKeyOptions) SetCorrelationID(correlationID string) *CreateKeyOptions {
	_options.CorrelationID = core.StringPtr(correlationID)
	return _options
}

// SetPrefer : Allow user to set Prefer
func (_options *CreateKeyOptions) SetPrefer(prefer string) *CreateKeyOptions {
	_options.Prefer = core.StringPtr(prefer)
	return _options
}

// SetXKmsKeyRing : Allow user to set XKmsKeyRing
func (_options *CreateKeyOptions) SetXKmsKeyRing(xKmsKeyRing string) *CreateKeyOptions {
	_options.XKmsKeyRing = core.StringPtr(xKmsKeyRing)
	return _options
}

// SetHeaders : Allow user to set Headers
func (options *CreateKeyOptions) SetHeaders(param map[string]string) *CreateKeyOptions {
	options.Headers = param
	return options
}

// CreateKeyRingOptions : The CreateKeyRing options.
type CreateKeyRingOptions struct {
	// The ID that identifies the key ring. Each ID is unique only within the given instance and is not reserved across the
	// Key Protect service.
	KeyRingID *string `json:"key-ring-id" validate:"required,ne="`

	// The IBM Cloud instance ID that identifies your Key Protect service instance.
	BluemixInstance *string `json:"Bluemix-Instance" validate:"required"`

	// The v4 UUID used to correlate and track transactions.
	CorrelationID *string `json:"Correlation-Id,omitempty"`

	// Allows users to set headers on API requests.
	Headers map[string]string
}

// NewCreateKeyRingOptions : Instantiate CreateKeyRingOptions
func (*IbmKeyProtectApiV2) NewCreateKeyRingOptions(keyRingID string, bluemixInstance string) *CreateKeyRingOptions {
	return &CreateKeyRingOptions{
		KeyRingID:       core.StringPtr(keyRingID),
		BluemixInstance: core.StringPtr(bluemixInstance),
	}
}

// SetKeyRingID : Allow user to set KeyRingID
func (_options *CreateKeyRingOptions) SetKeyRingID(keyRingID string) *CreateKeyRingOptions {
	_options.KeyRingID = core.StringPtr(keyRingID)
	return _options
}

// SetBluemixInstance : Allow user to set BluemixInstance
func (_options *CreateKeyRingOptions) SetBluemixInstance(bluemixInstance string) *CreateKeyRingOptions {
	_options.BluemixInstance = core.StringPtr(bluemixInstance)
	return _options
}

// SetCorrelationID : Allow user to set CorrelationID
func (_options *CreateKeyRingOptions) SetCorrelationID(correlationID string) *CreateKeyRingOptions {
	_options.CorrelationID = core.StringPtr(correlationID)
	return _options
}

// SetHeaders : Allow user to set Headers
func (options *CreateKeyRingOptions) SetHeaders(param map[string]string) *CreateKeyRingOptions {
	options.Headers = param
	return options
}

// CreateKeyWithPoliciesOverridesOptions : The CreateKeyWithPoliciesOverrides options.
type CreateKeyWithPoliciesOverridesOptions struct {
	// The IBM Cloud instance ID that identifies your Key Protect service instance.
	BluemixInstance *string `json:"Bluemix-Instance" validate:"required"`

	// The base request for creating a new key with policies.
	KeyWithPolicyOverridesCreateBody io.ReadCloser `json:"KeyWithPolicyOverridesCreateBody" validate:"required"`

	// The v4 UUID used to correlate and track transactions.
	CorrelationID *string `json:"Correlation-Id,omitempty"`

	// Alters server behavior for POST or DELETE operations. A header with `return=minimal` causes the service to return
	// only the key identifier as metadata. A header containing `return=representation` returns both the key material and
	// metadata in the response entity-body. If the key has been designated as a root key, the system cannot return the key
	// material.
	// **Note:** During POST operations, Key Protect may not immediately return the key material due to key generation
	// time. To retrieve the key material, you can perform a subsequent `GET /keys/{id}` request.
	Prefer *string `json:"Prefer,omitempty"`

	// The ID of the key ring that the specified key belongs to. When the header is not specified, Key Protect will perform
	// a key ring lookup. For a more optimized request, specify the key ring on every call. The key ring ID of keys that
	// are created without an `X-Kms-Key-Ring` header is: `default`.
	XKmsKeyRing *string `json:"X-Kms-Key-Ring,omitempty"`

	// Allows users to set headers on API requests.
	Headers map[string]string
}

// Constants associated with the CreateKeyWithPoliciesOverridesOptions.Prefer property.
// Alters server behavior for POST or DELETE operations. A header with `return=minimal` causes the service to return
// only the key identifier as metadata. A header containing `return=representation` returns both the key material and
// metadata in the response entity-body. If the key has been designated as a root key, the system cannot return the key
// material.
// **Note:** During POST operations, Key Protect may not immediately return the key material due to key generation time.
// To retrieve the key material, you can perform a subsequent `GET /keys/{id}` request.
const (
	CreateKeyWithPoliciesOverridesOptions_Prefer_ReturnMinimal        = "return=minimal"
	CreateKeyWithPoliciesOverridesOptions_Prefer_ReturnRepresentation = "return=representation"
)

// NewCreateKeyWithPoliciesOverridesOptions : Instantiate CreateKeyWithPoliciesOverridesOptions
func (*IbmKeyProtectApiV2) NewCreateKeyWithPoliciesOverridesOptions(bluemixInstance string, keyWithPolicyOverridesCreateBody io.ReadCloser) *CreateKeyWithPoliciesOverridesOptions {
	return &CreateKeyWithPoliciesOverridesOptions{
		BluemixInstance:                  core.StringPtr(bluemixInstance),
		KeyWithPolicyOverridesCreateBody: keyWithPolicyOverridesCreateBody,
	}
}

// SetBluemixInstance : Allow user to set BluemixInstance
func (_options *CreateKeyWithPoliciesOverridesOptions) SetBluemixInstance(bluemixInstance string) *CreateKeyWithPoliciesOverridesOptions {
	_options.BluemixInstance = core.StringPtr(bluemixInstance)
	return _options
}

// SetKeyWithPolicyOverridesCreateBody : Allow user to set KeyWithPolicyOverridesCreateBody
func (_options *CreateKeyWithPoliciesOverridesOptions) SetKeyWithPolicyOverridesCreateBody(keyWithPolicyOverridesCreateBody io.ReadCloser) *CreateKeyWithPoliciesOverridesOptions {
	_options.KeyWithPolicyOverridesCreateBody = keyWithPolicyOverridesCreateBody
	return _options
}

// SetCorrelationID : Allow user to set CorrelationID
func (_options *CreateKeyWithPoliciesOverridesOptions) SetCorrelationID(correlationID string) *CreateKeyWithPoliciesOverridesOptions {
	_options.CorrelationID = core.StringPtr(correlationID)
	return _options
}

// SetPrefer : Allow user to set Prefer
func (_options *CreateKeyWithPoliciesOverridesOptions) SetPrefer(prefer string) *CreateKeyWithPoliciesOverridesOptions {
	_options.Prefer = core.StringPtr(prefer)
	return _options
}

// SetXKmsKeyRing : Allow user to set XKmsKeyRing
func (_options *CreateKeyWithPoliciesOverridesOptions) SetXKmsKeyRing(xKmsKeyRing string) *CreateKeyWithPoliciesOverridesOptions {
	_options.XKmsKeyRing = core.StringPtr(xKmsKeyRing)
	return _options
}

// SetHeaders : Allow user to set Headers
func (options *CreateKeyWithPoliciesOverridesOptions) SetHeaders(param map[string]string) *CreateKeyWithPoliciesOverridesOptions {
	options.Headers = param
	return options
}

// CreateKmipAdapterOptions : The CreateKmipAdapter options.
type CreateKmipAdapterOptions struct {
	// The IBM Cloud instance ID that identifies your Key Protect service instance.
	BluemixInstance *string `json:"Bluemix-Instance" validate:"required"`

	// The metadata that describes the resource array.
	Metadata *CollectionMetadata `json:"metadata" validate:"required"`

	// A collection of resources.
	Resources []CreateKMIPAdapterObject `json:"resources" validate:"required"`

	// The v4 UUID used to correlate and track transactions.
	CorrelationID *string `json:"Correlation-Id,omitempty"`

	// If set to 'true', allows an active root key containing an expiration date to be associated with the KMIP adapter.
	AllowExpiringKey *bool `json:"allowExpiringKey,omitempty"`

	// Allows users to set headers on API requests.
	Headers map[string]string
}

// NewCreateKmipAdapterOptions : Instantiate CreateKmipAdapterOptions
func (*IbmKeyProtectApiV2) NewCreateKmipAdapterOptions(bluemixInstance string, metadata *CollectionMetadata, resources []CreateKMIPAdapterObject) *CreateKmipAdapterOptions {
	return &CreateKmipAdapterOptions{
		BluemixInstance: core.StringPtr(bluemixInstance),
		Metadata:        metadata,
		Resources:       resources,
	}
}

// SetBluemixInstance : Allow user to set BluemixInstance
func (_options *CreateKmipAdapterOptions) SetBluemixInstance(bluemixInstance string) *CreateKmipAdapterOptions {
	_options.BluemixInstance = core.StringPtr(bluemixInstance)
	return _options
}

// SetMetadata : Allow user to set Metadata
func (_options *CreateKmipAdapterOptions) SetMetadata(metadata *CollectionMetadata) *CreateKmipAdapterOptions {
	_options.Metadata = metadata
	return _options
}

// SetResources : Allow user to set Resources
func (_options *CreateKmipAdapterOptions) SetResources(resources []CreateKMIPAdapterObject) *CreateKmipAdapterOptions {
	_options.Resources = resources
	return _options
}

// SetCorrelationID : Allow user to set CorrelationID
func (_options *CreateKmipAdapterOptions) SetCorrelationID(correlationID string) *CreateKmipAdapterOptions {
	_options.CorrelationID = core.StringPtr(correlationID)
	return _options
}

// SetAllowExpiringKey : Allow user to set AllowExpiringKey
func (_options *CreateKmipAdapterOptions) SetAllowExpiringKey(allowExpiringKey bool) *CreateKmipAdapterOptions {
	_options.AllowExpiringKey = core.BoolPtr(allowExpiringKey)
	return _options
}

// SetHeaders : Allow user to set Headers
func (options *CreateKmipAdapterOptions) SetHeaders(param map[string]string) *CreateKmipAdapterOptions {
	options.Headers = param
	return options
}

// DeleteKey : The base schema for deleting keys.
type DeleteKey struct {
	// The metadata that describes the resource array.
	Metadata *CollectionMetadata `json:"metadata" validate:"required"`

	// A collection of resources.
	Resources []KeyWithPayload `json:"resources" validate:"required"`
}

// UnmarshalDeleteKey unmarshals an instance of DeleteKey from the specified map of raw messages.
func UnmarshalDeleteKey(m map[string]json.RawMessage, result interface{}) (err error) {
	obj := new(DeleteKey)
	err = core.UnmarshalModel(m, "metadata", &obj.Metadata, UnmarshalCollectionMetadata)
	if err != nil {
		err = core.SDKErrorf(err, "", "metadata-error", common.GetComponentInfo())
		return
	}
	err = core.UnmarshalModel(m, "resources", &obj.Resources, UnmarshalKeyWithPayload)
	if err != nil {
		err = core.SDKErrorf(err, "", "resources-error", common.GetComponentInfo())
		return
	}
	reflect.ValueOf(result).Elem().Set(reflect.ValueOf(obj))
	return
}

// DeleteKeyAliasOptions : The DeleteKeyAlias options.
type DeleteKeyAliasOptions struct {
	// The v4 UUID or alias that uniquely identifies the key.
	ID *string `json:"id" validate:"required,ne="`

	// A human-readable alias that uniquely identifies a key. Each alias is unique only within the given instance and is
	// not reserved across the Key Protect service. Each key can have up to five aliases. There is no limit to the number
	// of aliases per instance. The length of the alias can be between 2 - 90 characters, inclusive. An alias must be
	// alphanumeric and cannot contain spaces or special characters other than '-' or '_'. Also, the alias cannot be a
	// version 4 UUID and must not be a Key Protect reserved name: `allowed_ip`, `key`, `keys`, `metadata`, `policy`,
	// `policies`, `registration`, `registrations`, `ring`, `rings`, `rotate`, `wrap`, `unwrap`, `rewrap`, `version`,
	// `versions`.
	Alias *string `json:"alias" validate:"required,ne="`

	// The IBM Cloud instance ID that identifies your Key Protect service instance.
	BluemixInstance *string `json:"Bluemix-Instance" validate:"required"`

	// The v4 UUID used to correlate and track transactions.
	CorrelationID *string `json:"Correlation-Id,omitempty"`

	// The ID of the key ring that the specified key is a part of. When the header is not specified, Key Protect will
	// perform a key ring lookup. For a more optimized request, specify the key ring on every call. The key ring ID of keys
	// that are created without an `X-Kms-Key-Ring` header is: `default`.
	XKmsKeyRing *string `json:"X-Kms-Key-Ring,omitempty"`

	// Allows users to set headers on API requests.
	Headers map[string]string
}

// NewDeleteKeyAliasOptions : Instantiate DeleteKeyAliasOptions
func (*IbmKeyProtectApiV2) NewDeleteKeyAliasOptions(id string, alias string, bluemixInstance string) *DeleteKeyAliasOptions {
	return &DeleteKeyAliasOptions{
		ID:              core.StringPtr(id),
		Alias:           core.StringPtr(alias),
		BluemixInstance: core.StringPtr(bluemixInstance),
	}
}

// SetID : Allow user to set ID
func (_options *DeleteKeyAliasOptions) SetID(id string) *DeleteKeyAliasOptions {
	_options.ID = core.StringPtr(id)
	return _options
}

// SetAlias : Allow user to set Alias
func (_options *DeleteKeyAliasOptions) SetAlias(alias string) *DeleteKeyAliasOptions {
	_options.Alias = core.StringPtr(alias)
	return _options
}

// SetBluemixInstance : Allow user to set BluemixInstance
func (_options *DeleteKeyAliasOptions) SetBluemixInstance(bluemixInstance string) *DeleteKeyAliasOptions {
	_options.BluemixInstance = core.StringPtr(bluemixInstance)
	return _options
}

// SetCorrelationID : Allow user to set CorrelationID
func (_options *DeleteKeyAliasOptions) SetCorrelationID(correlationID string) *DeleteKeyAliasOptions {
	_options.CorrelationID = core.StringPtr(correlationID)
	return _options
}

// SetXKmsKeyRing : Allow user to set XKmsKeyRing
func (_options *DeleteKeyAliasOptions) SetXKmsKeyRing(xKmsKeyRing string) *DeleteKeyAliasOptions {
	_options.XKmsKeyRing = core.StringPtr(xKmsKeyRing)
	return _options
}

// SetHeaders : Allow user to set Headers
func (options *DeleteKeyAliasOptions) SetHeaders(param map[string]string) *DeleteKeyAliasOptions {
	options.Headers = param
	return options
}

// DeleteKeyOptions : The DeleteKey options.
type DeleteKeyOptions struct {
	// The v4 UUID that uniquely identifies the key.
	ID *string `json:"id" validate:"required,ne="`

	// The IBM Cloud instance ID that identifies your Key Protect service instance.
	BluemixInstance *string `json:"Bluemix-Instance" validate:"required"`

	// The v4 UUID used to correlate and track transactions.
	CorrelationID *string `json:"Correlation-Id,omitempty"`

	// The ID of the key ring that the specified key is a part of. When the header is not specified, Key Protect will
	// perform a key ring lookup. For a more optimized request, specify the key ring on every call. The key ring ID of keys
	// that are created without an `X-Kms-Key-Ring` header is: `default`.
	XKmsKeyRing *string `json:"X-Kms-Key-Ring,omitempty"`

	// Alters server behavior for POST or DELETE operations. A header with `return=minimal` causes the service to return
	// only the key identifier as metadata. A header containing `return=representation` returns both the key material and
	// metadata in the response entity-body. If the key has been designated as a root key, the system cannot return the key
	// material.
	// **Note:** During POST operations, Key Protect may not immediately return the key material due to key generation
	// time. To retrieve the key material, you can perform a subsequent `GET /keys/{id}` request.
	Prefer *string `json:"Prefer,omitempty"`

	// If set to `true`, Key Protect forces deletion on a key that is protecting a cloud resource, such as a Cloud Object
	// Storage bucket. The action removes any registrations that are associated with the key.
	// **Note:** If a key is protecting a cloud resource that has a retention policy, Key Protect cannot delete the key.
	// Use `GET keys/{id}/registrations` to review registrations between the key and its associated cloud resources. To
	// enable deletion, contact an account owner to remove the retention policy on each resource that is associated with
	// this key.
	Force *bool `json:"force,omitempty"`

	// Allows users to set headers on API requests.
	Headers map[string]string
}

// Constants associated with the DeleteKeyOptions.Prefer property.
// Alters server behavior for POST or DELETE operations. A header with `return=minimal` causes the service to return
// only the key identifier as metadata. A header containing `return=representation` returns both the key material and
// metadata in the response entity-body. If the key has been designated as a root key, the system cannot return the key
// material.
// **Note:** During POST operations, Key Protect may not immediately return the key material due to key generation time.
// To retrieve the key material, you can perform a subsequent `GET /keys/{id}` request.
const (
	DeleteKeyOptions_Prefer_ReturnMinimal        = "return=minimal"
	DeleteKeyOptions_Prefer_ReturnRepresentation = "return=representation"
)

// NewDeleteKeyOptions : Instantiate DeleteKeyOptions
func (*IbmKeyProtectApiV2) NewDeleteKeyOptions(id string, bluemixInstance string) *DeleteKeyOptions {
	return &DeleteKeyOptions{
		ID:              core.StringPtr(id),
		BluemixInstance: core.StringPtr(bluemixInstance),
	}
}

// SetID : Allow user to set ID
func (_options *DeleteKeyOptions) SetID(id string) *DeleteKeyOptions {
	_options.ID = core.StringPtr(id)
	return _options
}

// SetBluemixInstance : Allow user to set BluemixInstance
func (_options *DeleteKeyOptions) SetBluemixInstance(bluemixInstance string) *DeleteKeyOptions {
	_options.BluemixInstance = core.StringPtr(bluemixInstance)
	return _options
}

// SetCorrelationID : Allow user to set CorrelationID
func (_options *DeleteKeyOptions) SetCorrelationID(correlationID string) *DeleteKeyOptions {
	_options.CorrelationID = core.StringPtr(correlationID)
	return _options
}

// SetXKmsKeyRing : Allow user to set XKmsKeyRing
func (_options *DeleteKeyOptions) SetXKmsKeyRing(xKmsKeyRing string) *DeleteKeyOptions {
	_options.XKmsKeyRing = core.StringPtr(xKmsKeyRing)
	return _options
}

// SetPrefer : Allow user to set Prefer
func (_options *DeleteKeyOptions) SetPrefer(prefer string) *DeleteKeyOptions {
	_options.Prefer = core.StringPtr(prefer)
	return _options
}

// SetForce : Allow user to set Force
func (_options *DeleteKeyOptions) SetForce(force bool) *DeleteKeyOptions {
	_options.Force = core.BoolPtr(force)
	return _options
}

// SetHeaders : Allow user to set Headers
func (options *DeleteKeyOptions) SetHeaders(param map[string]string) *DeleteKeyOptions {
	options.Headers = param
	return options
}

// DeleteKeyRingOptions : The DeleteKeyRing options.
type DeleteKeyRingOptions struct {
	// The ID that identifies the key ring. Each ID is unique only within the given instance and is not reserved across the
	// Key Protect service.
	KeyRingID *string `json:"key-ring-id" validate:"required,ne="`

	// The IBM Cloud instance ID that identifies your Key Protect service instance.
	BluemixInstance *string `json:"Bluemix-Instance" validate:"required"`

	// The v4 UUID used to correlate and track transactions.
	CorrelationID *string `json:"Correlation-Id,omitempty"`

	// Force delete the key ring. All keys in the key ring are required to be deleted (in state `5`) before this action can
	// be performed. If the key ring to be deleted contains keys, they will be moved to the `default` key ring which
	// requires the `kms.secrets.patch` IAM action.
	Force *bool `json:"force,omitempty"`

	// Allows users to set headers on API requests.
	Headers map[string]string
}

// NewDeleteKeyRingOptions : Instantiate DeleteKeyRingOptions
func (*IbmKeyProtectApiV2) NewDeleteKeyRingOptions(keyRingID string, bluemixInstance string) *DeleteKeyRingOptions {
	return &DeleteKeyRingOptions{
		KeyRingID:       core.StringPtr(keyRingID),
		BluemixInstance: core.StringPtr(bluemixInstance),
	}
}

// SetKeyRingID : Allow user to set KeyRingID
func (_options *DeleteKeyRingOptions) SetKeyRingID(keyRingID string) *DeleteKeyRingOptions {
	_options.KeyRingID = core.StringPtr(keyRingID)
	return _options
}

// SetBluemixInstance : Allow user to set BluemixInstance
func (_options *DeleteKeyRingOptions) SetBluemixInstance(bluemixInstance string) *DeleteKeyRingOptions {
	_options.BluemixInstance = core.StringPtr(bluemixInstance)
	return _options
}

// SetCorrelationID : Allow user to set CorrelationID
func (_options *DeleteKeyRingOptions) SetCorrelationID(correlationID string) *DeleteKeyRingOptions {
	_options.CorrelationID = core.StringPtr(correlationID)
	return _options
}

// SetForce : Allow user to set Force
func (_options *DeleteKeyRingOptions) SetForce(force bool) *DeleteKeyRingOptions {
	_options.Force = core.BoolPtr(force)
	return _options
}

// SetHeaders : Allow user to set Headers
func (options *DeleteKeyRingOptions) SetHeaders(param map[string]string) *DeleteKeyRingOptions {
	options.Headers = param
	return options
}

// DeleteKmipAdapterOptions : The DeleteKmipAdapter options.
type DeleteKmipAdapterOptions struct {
	// The name or v4 UUID of the KMIP Adapter that uniquely identifies it.
	ID *string `json:"id" validate:"required,ne="`

	// The IBM Cloud instance ID that identifies your Key Protect service instance.
	BluemixInstance *string `json:"Bluemix-Instance" validate:"required"`

	// The v4 UUID used to correlate and track transactions.
	CorrelationID *string `json:"Correlation-Id,omitempty"`

	// Allows users to set headers on API requests.
	Headers map[string]string
}

// NewDeleteKmipAdapterOptions : Instantiate DeleteKmipAdapterOptions
func (*IbmKeyProtectApiV2) NewDeleteKmipAdapterOptions(id string, bluemixInstance string) *DeleteKmipAdapterOptions {
	return &DeleteKmipAdapterOptions{
		ID:              core.StringPtr(id),
		BluemixInstance: core.StringPtr(bluemixInstance),
	}
}

// SetID : Allow user to set ID
func (_options *DeleteKmipAdapterOptions) SetID(id string) *DeleteKmipAdapterOptions {
	_options.ID = core.StringPtr(id)
	return _options
}

// SetBluemixInstance : Allow user to set BluemixInstance
func (_options *DeleteKmipAdapterOptions) SetBluemixInstance(bluemixInstance string) *DeleteKmipAdapterOptions {
	_options.BluemixInstance = core.StringPtr(bluemixInstance)
	return _options
}

// SetCorrelationID : Allow user to set CorrelationID
func (_options *DeleteKmipAdapterOptions) SetCorrelationID(correlationID string) *DeleteKmipAdapterOptions {
	_options.CorrelationID = core.StringPtr(correlationID)
	return _options
}

// SetHeaders : Allow user to set Headers
func (options *DeleteKmipAdapterOptions) SetHeaders(param map[string]string) *DeleteKmipAdapterOptions {
	options.Headers = param
	return options
}

// DeleteKmipClientCertificateOptions : The DeleteKmipClientCertificate options.
type DeleteKmipClientCertificateOptions struct {
	// The name or v4 UUID of the KMIP Adapter that uniquely identifies it.
	AdapterID *string `json:"adapter_id" validate:"required,ne="`

	// The name or v4 UUID of the client certificate that uniquely identifies it.
	ID *string `json:"id" validate:"required,ne="`

	// The IBM Cloud instance ID that identifies your Key Protect service instance.
	BluemixInstance *string `json:"Bluemix-Instance" validate:"required"`

	// The v4 UUID used to correlate and track transactions.
	CorrelationID *string `json:"Correlation-Id,omitempty"`

	// Allows users to set headers on API requests.
	Headers map[string]string
}

// NewDeleteKmipClientCertificateOptions : Instantiate DeleteKmipClientCertificateOptions
func (*IbmKeyProtectApiV2) NewDeleteKmipClientCertificateOptions(adapterID string, id string, bluemixInstance string) *DeleteKmipClientCertificateOptions {
	return &DeleteKmipClientCertificateOptions{
		AdapterID:       core.StringPtr(adapterID),
		ID:              core.StringPtr(id),
		BluemixInstance: core.StringPtr(bluemixInstance),
	}
}

// SetAdapterID : Allow user to set AdapterID
func (_options *DeleteKmipClientCertificateOptions) SetAdapterID(adapterID string) *DeleteKmipClientCertificateOptions {
	_options.AdapterID = core.StringPtr(adapterID)
	return _options
}

// SetID : Allow user to set ID
func (_options *DeleteKmipClientCertificateOptions) SetID(id string) *DeleteKmipClientCertificateOptions {
	_options.ID = core.StringPtr(id)
	return _options
}

// SetBluemixInstance : Allow user to set BluemixInstance
func (_options *DeleteKmipClientCertificateOptions) SetBluemixInstance(bluemixInstance string) *DeleteKmipClientCertificateOptions {
	_options.BluemixInstance = core.StringPtr(bluemixInstance)
	return _options
}

// SetCorrelationID : Allow user to set CorrelationID
func (_options *DeleteKmipClientCertificateOptions) SetCorrelationID(correlationID string) *DeleteKmipClientCertificateOptions {
	_options.CorrelationID = core.StringPtr(correlationID)
	return _options
}

// SetHeaders : Allow user to set Headers
func (options *DeleteKmipClientCertificateOptions) SetHeaders(param map[string]string) *DeleteKmipClientCertificateOptions {
	options.Headers = param
	return options
}

// DeleteKmipObjectOptions : The DeleteKmipObject options.
type DeleteKmipObjectOptions struct {
	// The name or v4 UUID of the KMIP Adapter that uniquely identifies it.
	AdapterID *string `json:"adapter_id" validate:"required,ne="`

	// The IBM Cloud instance ID that identifies your Key Protect service instance.
	BluemixInstance *string `json:"Bluemix-Instance" validate:"required"`

	// The name or v4 UUID of the client certificate that uniquely identifies it.
	ID *string `json:"id" validate:"required,ne="`

	// The v4 UUID used to correlate and track transactions.
	CorrelationID *string `json:"Correlation-Id,omitempty"`

	// Force delete the KMIP object, regardless of the object's state. All object data is eligible to be purged 90 days
	// after deletion.
	Force *bool `json:"force,omitempty"`

	// Allows users to set headers on API requests.
	Headers map[string]string
}

// NewDeleteKmipObjectOptions : Instantiate DeleteKmipObjectOptions
func (*IbmKeyProtectApiV2) NewDeleteKmipObjectOptions(adapterID string, bluemixInstance string, id string) *DeleteKmipObjectOptions {
	return &DeleteKmipObjectOptions{
		AdapterID:       core.StringPtr(adapterID),
		BluemixInstance: core.StringPtr(bluemixInstance),
		ID:              core.StringPtr(id),
	}
}

// SetAdapterID : Allow user to set AdapterID
func (_options *DeleteKmipObjectOptions) SetAdapterID(adapterID string) *DeleteKmipObjectOptions {
	_options.AdapterID = core.StringPtr(adapterID)
	return _options
}

// SetBluemixInstance : Allow user to set BluemixInstance
func (_options *DeleteKmipObjectOptions) SetBluemixInstance(bluemixInstance string) *DeleteKmipObjectOptions {
	_options.BluemixInstance = core.StringPtr(bluemixInstance)
	return _options
}

// SetID : Allow user to set ID
func (_options *DeleteKmipObjectOptions) SetID(id string) *DeleteKmipObjectOptions {
	_options.ID = core.StringPtr(id)
	return _options
}

// SetCorrelationID : Allow user to set CorrelationID
func (_options *DeleteKmipObjectOptions) SetCorrelationID(correlationID string) *DeleteKmipObjectOptions {
	_options.CorrelationID = core.StringPtr(correlationID)
	return _options
}

// SetForce : Allow user to set Force
func (_options *DeleteKmipObjectOptions) SetForce(force bool) *DeleteKmipObjectOptions {
	_options.Force = core.BoolPtr(force)
	return _options
}

// SetHeaders : Allow user to set Headers
func (options *DeleteKmipObjectOptions) SetHeaders(param map[string]string) *DeleteKmipObjectOptions {
	options.Headers = param
	return options
}

// DisableKeyOptions : The DisableKey options.
type DisableKeyOptions struct {
	// The v4 UUID or alias that uniquely identifies the key.
	ID *string `json:"id" validate:"required,ne="`

	// The IBM Cloud instance ID that identifies your Key Protect service instance.
	BluemixInstance *string `json:"Bluemix-Instance" validate:"required"`

	// The v4 UUID used to correlate and track transactions.
	CorrelationID *string `json:"Correlation-Id,omitempty"`

	// The ID of the key ring that the specified key is a part of. When the header is not specified, Key Protect will
	// perform a key ring lookup. For a more optimized request, specify the key ring on every call. The key ring ID of keys
	// that are created without an `X-Kms-Key-Ring` header is: `default`.
	XKmsKeyRing *string `json:"X-Kms-Key-Ring,omitempty"`

	// Allows users to set headers on API requests.
	Headers map[string]string
}

// NewDisableKeyOptions : Instantiate DisableKeyOptions
func (*IbmKeyProtectApiV2) NewDisableKeyOptions(id string, bluemixInstance string) *DisableKeyOptions {
	return &DisableKeyOptions{
		ID:              core.StringPtr(id),
		BluemixInstance: core.StringPtr(bluemixInstance),
	}
}

// SetID : Allow user to set ID
func (_options *DisableKeyOptions) SetID(id string) *DisableKeyOptions {
	_options.ID = core.StringPtr(id)
	return _options
}

// SetBluemixInstance : Allow user to set BluemixInstance
func (_options *DisableKeyOptions) SetBluemixInstance(bluemixInstance string) *DisableKeyOptions {
	_options.BluemixInstance = core.StringPtr(bluemixInstance)
	return _options
}

// SetCorrelationID : Allow user to set CorrelationID
func (_options *DisableKeyOptions) SetCorrelationID(correlationID string) *DisableKeyOptions {
	_options.CorrelationID = core.StringPtr(correlationID)
	return _options
}

// SetXKmsKeyRing : Allow user to set XKmsKeyRing
func (_options *DisableKeyOptions) SetXKmsKeyRing(xKmsKeyRing string) *DisableKeyOptions {
	_options.XKmsKeyRing = core.StringPtr(xKmsKeyRing)
	return _options
}

// SetHeaders : Allow user to set Headers
func (options *DisableKeyOptions) SetHeaders(param map[string]string) *DisableKeyOptions {
	options.Headers = param
	return options
}

// DualAuthDeleteProperties : User defined metadata that is associated with the `dualAuthDelete` instance policy type.
type DualAuthDeleteProperties struct {
	// If set to `true`, Key Protect enables a dual authorization deletion policy for your service instance. By default,
	// Key Protect requires only one authorization to delete a key. After you enable a dual authorization policy, any new
	// key that you create or add to the instance will require an authorization from two users to delete keys.
	// **Note:** This change does not affect existing keys in your instance.
	Enabled *bool `json:"enabled" validate:"required"`
}

// NewDualAuthDeleteProperties : Instantiate DualAuthDeleteProperties (Generic Model Constructor)
func (*IbmKeyProtectApiV2) NewDualAuthDeleteProperties(enabled bool) (_model *DualAuthDeleteProperties, err error) {
	_model = &DualAuthDeleteProperties{
		Enabled: core.BoolPtr(enabled),
	}
	err = core.ValidateStruct(_model, "required parameters")
	if err != nil {
		err = core.SDKErrorf(err, "", "model-missing-required", common.GetComponentInfo())
	}
	return
}

// UnmarshalDualAuthDeleteProperties unmarshals an instance of DualAuthDeleteProperties from the specified map of raw messages.
func UnmarshalDualAuthDeleteProperties(m map[string]json.RawMessage, result interface{}) (err error) {
	obj := new(DualAuthDeleteProperties)
	err = core.UnmarshalPrimitive(m, "enabled", &obj.Enabled)
	if err != nil {
		err = core.SDKErrorf(err, "", "enabled-error", common.GetComponentInfo())
		return
	}
	reflect.ValueOf(result).Elem().Set(reflect.ValueOf(obj))
	return
}

// DualAuthKeyMetadata : Metadata that indicates the status of a dual authorization policy on the key.
type DualAuthKeyMetadata struct {
	// The status of a dual authorization policy on the key. If `true`, dual authorization is required to delete the key.
	// If `false`, no prior authorization is required to delete the key.
	Enabled *bool `json:"enabled" validate:"required"`

	// Indicates if a delete authorization has been issued for a key. If `true`, an authorization to delete this key has
	// been issued by the first user, and a second user with a Manager access policy can safely delete the key. If the
	// `enabled` property is `false`, this field is omitted in the response body.
	KeySetForDeletion *bool `json:"keySetForDeletion,omitempty"`

	// The date that an authorization for deletion expires for the key. If this date has passed, the authorization is no
	// longer valid. If the `enabled` or `keySetForDeletion` properties are `false`, this field is omitted in the response
	// body.
	AuthExpiration *strfmt.DateTime `json:"authExpiration,omitempty"`
}

// UnmarshalDualAuthKeyMetadata unmarshals an instance of DualAuthKeyMetadata from the specified map of raw messages.
func UnmarshalDualAuthKeyMetadata(m map[string]json.RawMessage, result interface{}) (err error) {
	obj := new(DualAuthKeyMetadata)
	err = core.UnmarshalPrimitive(m, "enabled", &obj.Enabled)
	if err != nil {
		err = core.SDKErrorf(err, "", "enabled-error", common.GetComponentInfo())
		return
	}
	err = core.UnmarshalPrimitive(m, "keySetForDeletion", &obj.KeySetForDeletion)
	if err != nil {
		err = core.SDKErrorf(err, "", "keySetForDeletion-error", common.GetComponentInfo())
		return
	}
	err = core.UnmarshalPrimitive(m, "authExpiration", &obj.AuthExpiration)
	if err != nil {
		err = core.SDKErrorf(err, "", "authExpiration-error", common.GetComponentInfo())
		return
	}
	reflect.ValueOf(result).Elem().Set(reflect.ValueOf(obj))
	return
}

// EnableKeyOptions : The EnableKey options.
type EnableKeyOptions struct {
	// The v4 UUID or alias that uniquely identifies the key.
	ID *string `json:"id" validate:"required,ne="`

	// The IBM Cloud instance ID that identifies your Key Protect service instance.
	BluemixInstance *string `json:"Bluemix-Instance" validate:"required"`

	// The v4 UUID used to correlate and track transactions.
	CorrelationID *string `json:"Correlation-Id,omitempty"`

	// The ID of the key ring that the specified key is a part of. When the header is not specified, Key Protect will
	// perform a key ring lookup. For a more optimized request, specify the key ring on every call. The key ring ID of keys
	// that are created without an `X-Kms-Key-Ring` header is: `default`.
	XKmsKeyRing *string `json:"X-Kms-Key-Ring,omitempty"`

	// Allows users to set headers on API requests.
	Headers map[string]string
}

// NewEnableKeyOptions : Instantiate EnableKeyOptions
func (*IbmKeyProtectApiV2) NewEnableKeyOptions(id string, bluemixInstance string) *EnableKeyOptions {
	return &EnableKeyOptions{
		ID:              core.StringPtr(id),
		BluemixInstance: core.StringPtr(bluemixInstance),
	}
}

// SetID : Allow user to set ID
func (_options *EnableKeyOptions) SetID(id string) *EnableKeyOptions {
	_options.ID = core.StringPtr(id)
	return _options
}

// SetBluemixInstance : Allow user to set BluemixInstance
func (_options *EnableKeyOptions) SetBluemixInstance(bluemixInstance string) *EnableKeyOptions {
	_options.BluemixInstance = core.StringPtr(bluemixInstance)
	return _options
}

// SetCorrelationID : Allow user to set CorrelationID
func (_options *EnableKeyOptions) SetCorrelationID(correlationID string) *EnableKeyOptions {
	_options.CorrelationID = core.StringPtr(correlationID)
	return _options
}

// SetXKmsKeyRing : Allow user to set XKmsKeyRing
func (_options *EnableKeyOptions) SetXKmsKeyRing(xKmsKeyRing string) *EnableKeyOptions {
	_options.XKmsKeyRing = core.StringPtr(xKmsKeyRing)
	return _options
}

// SetHeaders : Allow user to set Headers
func (options *EnableKeyOptions) SetHeaders(param map[string]string) *EnableKeyOptions {
	options.Headers = param
	return options
}

// GetAllowedIPPortOptions : The GetAllowedIPPort options.
type GetAllowedIPPortOptions struct {
	// The IBM Cloud instance ID that identifies your Key Protect service instance.
	BluemixInstance *string `json:"Bluemix-Instance" validate:"required"`

	// The v4 UUID used to correlate and track transactions.
	CorrelationID *string `json:"Correlation-Id,omitempty"`

	// Allows users to set headers on API requests.
	Headers map[string]string
}

// NewGetAllowedIPPortOptions : Instantiate GetAllowedIPPortOptions
func (*IbmKeyProtectApiV2) NewGetAllowedIPPortOptions(bluemixInstance string) *GetAllowedIPPortOptions {
	return &GetAllowedIPPortOptions{
		BluemixInstance: core.StringPtr(bluemixInstance),
	}
}

// SetBluemixInstance : Allow user to set BluemixInstance
func (_options *GetAllowedIPPortOptions) SetBluemixInstance(bluemixInstance string) *GetAllowedIPPortOptions {
	_options.BluemixInstance = core.StringPtr(bluemixInstance)
	return _options
}

// SetCorrelationID : Allow user to set CorrelationID
func (_options *GetAllowedIPPortOptions) SetCorrelationID(correlationID string) *GetAllowedIPPortOptions {
	_options.CorrelationID = core.StringPtr(correlationID)
	return _options
}

// SetHeaders : Allow user to set Headers
func (options *GetAllowedIPPortOptions) SetHeaders(param map[string]string) *GetAllowedIPPortOptions {
	options.Headers = param
	return options
}

// GetImportToken : The base schema for retrieving an import token.
type GetImportToken struct {
	// The time in seconds from the creation of an import token that determines how long its associated public key remains
	// valid. The minimum value is `300` seconds (5 minutes), and the maximum value is `86400` (24 hours). The default
	// value is `600` (10 minutes).
	Expiration *float64 `json:"expiration,omitempty"`

	// The number of times that an import token can be retrieved within its expiration time before it is no longer
	// accessible.
	MaxAllowedRetrievals *float64 `json:"maxAllowedRetrievals,omitempty"`

	// The date the import token was created. The date format follows RFC 3339.
	CreationDate *strfmt.DateTime `json:"creationDate,omitempty"`

	// The date the import token expires. The date format follows RFC 3339.
	ExpirationDate *strfmt.DateTime `json:"expirationDate,omitempty"`

	// The number of retrievals that are available for the import token before it is no longer accessible.
	RemainingRetrievals *float64 `json:"remainingRetrievals,omitempty"`

	// The public encryption key that you can use to encrypt key material before you import it into the service. This value
	// is a PEM-encoded public key in PKIX format. Because PEM encoding is a binary format, the value is base64 encoded.
	Payload *[]byte `json:"payload,omitempty"`

	// The nonce value that is used to verify a key import request. Encrypt and provide the encrypted nonce value when you
	// use `POST /keys` to securely import a key to the service.
	Nonce *[]byte `json:"nonce,omitempty"`
}

// UnmarshalGetImportToken unmarshals an instance of GetImportToken from the specified map of raw messages.
func UnmarshalGetImportToken(m map[string]json.RawMessage, result interface{}) (err error) {
	obj := new(GetImportToken)
	err = core.UnmarshalPrimitive(m, "expiration", &obj.Expiration)
	if err != nil {
		err = core.SDKErrorf(err, "", "expiration-error", common.GetComponentInfo())
		return
	}
	err = core.UnmarshalPrimitive(m, "maxAllowedRetrievals", &obj.MaxAllowedRetrievals)
	if err != nil {
		err = core.SDKErrorf(err, "", "maxAllowedRetrievals-error", common.GetComponentInfo())
		return
	}
	err = core.UnmarshalPrimitive(m, "creationDate", &obj.CreationDate)
	if err != nil {
		err = core.SDKErrorf(err, "", "creationDate-error", common.GetComponentInfo())
		return
	}
	err = core.UnmarshalPrimitive(m, "expirationDate", &obj.ExpirationDate)
	if err != nil {
		err = core.SDKErrorf(err, "", "expirationDate-error", common.GetComponentInfo())
		return
	}
	err = core.UnmarshalPrimitive(m, "remainingRetrievals", &obj.RemainingRetrievals)
	if err != nil {
		err = core.SDKErrorf(err, "", "remainingRetrievals-error", common.GetComponentInfo())
		return
	}
	err = core.UnmarshalPrimitive(m, "payload", &obj.Payload)
	if err != nil {
		err = core.SDKErrorf(err, "", "payload-error", common.GetComponentInfo())
		return
	}
	err = core.UnmarshalPrimitive(m, "nonce", &obj.Nonce)
	if err != nil {
		err = core.SDKErrorf(err, "", "nonce-error", common.GetComponentInfo())
		return
	}
	reflect.ValueOf(result).Elem().Set(reflect.ValueOf(obj))
	return
}

// GetImportTokenOptions : The GetImportToken options.
type GetImportTokenOptions struct {
	// The IBM Cloud instance ID that identifies your Key Protect service instance.
	BluemixInstance *string `json:"Bluemix-Instance" validate:"required"`

	// The v4 UUID used to correlate and track transactions.
	CorrelationID *string `json:"Correlation-Id,omitempty"`

	// The ID of the key ring that the specified key belongs to. When the header is not specified, Key Protect will perform
	// a key ring lookup. For a more optimized request, specify the key ring on every call. The key ring ID of keys that
	// are created without an `X-Kms-Key-Ring` header is: `default`.
	XKmsKeyRing *string `json:"X-Kms-Key-Ring,omitempty"`

	// Allows users to set headers on API requests.
	Headers map[string]string
}

// NewGetImportTokenOptions : Instantiate GetImportTokenOptions
func (*IbmKeyProtectApiV2) NewGetImportTokenOptions(bluemixInstance string) *GetImportTokenOptions {
	return &GetImportTokenOptions{
		BluemixInstance: core.StringPtr(bluemixInstance),
	}
}

// SetBluemixInstance : Allow user to set BluemixInstance
func (_options *GetImportTokenOptions) SetBluemixInstance(bluemixInstance string) *GetImportTokenOptions {
	_options.BluemixInstance = core.StringPtr(bluemixInstance)
	return _options
}

// SetCorrelationID : Allow user to set CorrelationID
func (_options *GetImportTokenOptions) SetCorrelationID(correlationID string) *GetImportTokenOptions {
	_options.CorrelationID = core.StringPtr(correlationID)
	return _options
}

// SetXKmsKeyRing : Allow user to set XKmsKeyRing
func (_options *GetImportTokenOptions) SetXKmsKeyRing(xKmsKeyRing string) *GetImportTokenOptions {
	_options.XKmsKeyRing = core.StringPtr(xKmsKeyRing)
	return _options
}

// SetHeaders : Allow user to set Headers
func (options *GetImportTokenOptions) SetHeaders(param map[string]string) *GetImportTokenOptions {
	options.Headers = param
	return options
}

// GetInstancePoliciesOneOf : GetInstancePoliciesOneOf struct
// Models which "extend" this model:
// - GetInstancePoliciesOneOfGetInstancePolicyAllowedNetwork
// - GetInstancePoliciesOneOfGetInstancePolicyDualAuthDelete
// - GetInstancePoliciesOneOfGetInstancePolicyAllowedIP
// - GetInstancePoliciesOneOfGetInstancePolicyKeyCreateImportAccess
// - GetInstancePoliciesOneOfGetInstancePolicyMetrics
// - GetInstancePoliciesOneOfGetInstancePolicyRotation
// - GetInstancePoliciesOneOfGetMultipleInstancePolicies
type GetInstancePoliciesOneOf struct {
	Metadata CollectionMetadataOneOfIntf `json:"metadata,omitempty"`

	// A collection of resources.
	Resources []GetInstancePoliciesOneOfResourcesItem `json:"resources,omitempty"`
}

func (*GetInstancePoliciesOneOf) isaGetInstancePoliciesOneOf() bool {
	return true
}

type GetInstancePoliciesOneOfIntf interface {
	isaGetInstancePoliciesOneOf() bool
}

// UnmarshalGetInstancePoliciesOneOf unmarshals an instance of GetInstancePoliciesOneOf from the specified map of raw messages.
func UnmarshalGetInstancePoliciesOneOf(m map[string]json.RawMessage, result interface{}) (err error) {
	obj := new(GetInstancePoliciesOneOf)
	err = core.UnmarshalModel(m, "metadata", &obj.Metadata, UnmarshalCollectionMetadataOneOf)
	if err != nil {
		err = core.SDKErrorf(err, "", "metadata-error", common.GetComponentInfo())
		return
	}
	err = core.UnmarshalModel(m, "resources", &obj.Resources, UnmarshalGetInstancePoliciesOneOfResourcesItem)
	if err != nil {
		err = core.SDKErrorf(err, "", "resources-error", common.GetComponentInfo())
		return
	}
	reflect.ValueOf(result).Elem().Set(reflect.ValueOf(obj))
	return
}

// GetInstancePoliciesOneOfGetInstancePolicyAllowedNetworkResourcesItem : GetInstancePoliciesOneOfGetInstancePolicyAllowedNetworkResourcesItem struct
type GetInstancePoliciesOneOfGetInstancePolicyAllowedNetworkResourcesItem struct {
	// The date the policy was created. The date format follows RFC 3339.
	CreationDate *strfmt.DateTime `json:"creationDate,omitempty"`

	// The unique identifier for the resource that created the policy.
	CreatedBy *string `json:"createdBy,omitempty"`

	// The unique identifier for the resource that updated the policy.
	UpdatedBy *string `json:"updatedBy,omitempty"`

	// Updates when the policy is replaced or modified. The date format follows RFC 3339.
	LastUpdated *strfmt.DateTime `json:"lastUpdated,omitempty"`

	// The type of policy to be retrieved.
	PolicyType *string `json:"policy_type" validate:"required"`

	// User defined metadata that is associated with the `allowedNetwork` instance policy type.
	PolicyData *GetInstancePoliciesOneOfGetInstancePolicyAllowedNetworkResourcesItemPolicyData `json:"policy_data" validate:"required"`
}

// UnmarshalGetInstancePoliciesOneOfGetInstancePolicyAllowedNetworkResourcesItem unmarshals an instance of GetInstancePoliciesOneOfGetInstancePolicyAllowedNetworkResourcesItem from the specified map of raw messages.
func UnmarshalGetInstancePoliciesOneOfGetInstancePolicyAllowedNetworkResourcesItem(m map[string]json.RawMessage, result interface{}) (err error) {
	obj := new(GetInstancePoliciesOneOfGetInstancePolicyAllowedNetworkResourcesItem)
	err = core.UnmarshalPrimitive(m, "creationDate", &obj.CreationDate)
	if err != nil {
		err = core.SDKErrorf(err, "", "creationDate-error", common.GetComponentInfo())
		return
	}
	err = core.UnmarshalPrimitive(m, "createdBy", &obj.CreatedBy)
	if err != nil {
		err = core.SDKErrorf(err, "", "createdBy-error", common.GetComponentInfo())
		return
	}
	err = core.UnmarshalPrimitive(m, "updatedBy", &obj.UpdatedBy)
	if err != nil {
		err = core.SDKErrorf(err, "", "updatedBy-error", common.GetComponentInfo())
		return
	}
	err = core.UnmarshalPrimitive(m, "lastUpdated", &obj.LastUpdated)
	if err != nil {
		err = core.SDKErrorf(err, "", "lastUpdated-error", common.GetComponentInfo())
		return
	}
	err = core.UnmarshalPrimitive(m, "policy_type", &obj.PolicyType)
	if err != nil {
		err = core.SDKErrorf(err, "", "policy_type-error", common.GetComponentInfo())
		return
	}
	err = core.UnmarshalModel(m, "policy_data", &obj.PolicyData, UnmarshalGetInstancePoliciesOneOfGetInstancePolicyAllowedNetworkResourcesItemPolicyData)
	if err != nil {
		err = core.SDKErrorf(err, "", "policy_data-error", common.GetComponentInfo())
		return
	}
	reflect.ValueOf(result).Elem().Set(reflect.ValueOf(obj))
	return
}

// GetInstancePoliciesOneOfGetInstancePolicyAllowedNetworkResourcesItemPolicyData : User defined metadata that is associated with the `allowedNetwork` instance policy type.
type GetInstancePoliciesOneOfGetInstancePolicyAllowedNetworkResourcesItemPolicyData struct {
	// If set to `true`, Key Protect enables the specified policy for your service instance. If set to `false`, Key Protect
	// disables the specified policy for your service instance, and the policy will no longer affect Key Protect actions.
	// **Note:** If a policy with attributes is disabled, all attributes are reset and are not retained.
	Enabled *bool `json:"enabled" validate:"required"`

	// Data associated with the policy type `allowed_network`.
	Attributes *GetInstancePoliciesOneOfGetInstancePolicyAllowedNetworkResourcesItemPolicyDataAttributes `json:"attributes,omitempty"`
}

// UnmarshalGetInstancePoliciesOneOfGetInstancePolicyAllowedNetworkResourcesItemPolicyData unmarshals an instance of GetInstancePoliciesOneOfGetInstancePolicyAllowedNetworkResourcesItemPolicyData from the specified map of raw messages.
func UnmarshalGetInstancePoliciesOneOfGetInstancePolicyAllowedNetworkResourcesItemPolicyData(m map[string]json.RawMessage, result interface{}) (err error) {
	obj := new(GetInstancePoliciesOneOfGetInstancePolicyAllowedNetworkResourcesItemPolicyData)
	err = core.UnmarshalPrimitive(m, "enabled", &obj.Enabled)
	if err != nil {
		err = core.SDKErrorf(err, "", "enabled-error", common.GetComponentInfo())
		return
	}
	err = core.UnmarshalModel(m, "attributes", &obj.Attributes, UnmarshalGetInstancePoliciesOneOfGetInstancePolicyAllowedNetworkResourcesItemPolicyDataAttributes)
	if err != nil {
		err = core.SDKErrorf(err, "", "attributes-error", common.GetComponentInfo())
		return
	}
	reflect.ValueOf(result).Elem().Set(reflect.ValueOf(obj))
	return
}

// GetInstancePoliciesOneOfGetInstancePolicyAllowedNetworkResourcesItemPolicyDataAttributes : Data associated with the policy type `allowed_network`.
type GetInstancePoliciesOneOfGetInstancePolicyAllowedNetworkResourcesItemPolicyDataAttributes struct {
	// If set to `public-and-private`, Key Protect allows the instance to be accessible through public and private
	// endpoints. If set to `private-only`, Key Protect restricts the instance to only be accessible through a private
	// endpoint.
	AllowedNetwork *string `json:"allowed_network" validate:"required"`
}

// Constants associated with the GetInstancePoliciesOneOfGetInstancePolicyAllowedNetworkResourcesItemPolicyDataAttributes.AllowedNetwork property.
// If set to `public-and-private`, Key Protect allows the instance to be accessible through public and private
// endpoints. If set to `private-only`, Key Protect restricts the instance to only be accessible through a private
// endpoint.
const (
	GetInstancePoliciesOneOfGetInstancePolicyAllowedNetworkResourcesItemPolicyDataAttributes_AllowedNetwork_PrivateOnly      = "private-only"
	GetInstancePoliciesOneOfGetInstancePolicyAllowedNetworkResourcesItemPolicyDataAttributes_AllowedNetwork_PublicAndPrivate = "public-and-private"
)

// UnmarshalGetInstancePoliciesOneOfGetInstancePolicyAllowedNetworkResourcesItemPolicyDataAttributes unmarshals an instance of GetInstancePoliciesOneOfGetInstancePolicyAllowedNetworkResourcesItemPolicyDataAttributes from the specified map of raw messages.
func UnmarshalGetInstancePoliciesOneOfGetInstancePolicyAllowedNetworkResourcesItemPolicyDataAttributes(m map[string]json.RawMessage, result interface{}) (err error) {
	obj := new(GetInstancePoliciesOneOfGetInstancePolicyAllowedNetworkResourcesItemPolicyDataAttributes)
	err = core.UnmarshalPrimitive(m, "allowed_network", &obj.AllowedNetwork)
	if err != nil {
		err = core.SDKErrorf(err, "", "allowed_network-error", common.GetComponentInfo())
		return
	}
	reflect.ValueOf(result).Elem().Set(reflect.ValueOf(obj))
	return
}

// GetInstancePoliciesOneOfGetInstancePolicyKeyCreateImportAccessResourcesItem : GetInstancePoliciesOneOfGetInstancePolicyKeyCreateImportAccessResourcesItem struct
type GetInstancePoliciesOneOfGetInstancePolicyKeyCreateImportAccessResourcesItem struct {
	// The date the policy was created. The date format follows RFC 3339.
	CreationDate *strfmt.DateTime `json:"creationDate,omitempty"`

	// The unique identifier for the resource that created the policy.
	CreatedBy *string `json:"createdBy,omitempty"`

	// The unique identifier for the resource that updated the policy.
	UpdatedBy *string `json:"updatedBy,omitempty"`

	// Updates when the policy is replaced or modified. The date format follows RFC 3339.
	LastUpdated *strfmt.DateTime `json:"lastUpdated,omitempty"`

	// The type of policy to be retrieved.
	PolicyType *string `json:"policy_type" validate:"required"`

	// User defined metadata that is associated with the `keyCreateImportAccess` instance policy type.
	PolicyData *GetInstancePoliciesOneOfGetInstancePolicyKeyCreateImportAccessResourcesItemPolicyData `json:"policy_data" validate:"required"`
}

// UnmarshalGetInstancePoliciesOneOfGetInstancePolicyKeyCreateImportAccessResourcesItem unmarshals an instance of GetInstancePoliciesOneOfGetInstancePolicyKeyCreateImportAccessResourcesItem from the specified map of raw messages.
func UnmarshalGetInstancePoliciesOneOfGetInstancePolicyKeyCreateImportAccessResourcesItem(m map[string]json.RawMessage, result interface{}) (err error) {
	obj := new(GetInstancePoliciesOneOfGetInstancePolicyKeyCreateImportAccessResourcesItem)
	err = core.UnmarshalPrimitive(m, "creationDate", &obj.CreationDate)
	if err != nil {
		err = core.SDKErrorf(err, "", "creationDate-error", common.GetComponentInfo())
		return
	}
	err = core.UnmarshalPrimitive(m, "createdBy", &obj.CreatedBy)
	if err != nil {
		err = core.SDKErrorf(err, "", "createdBy-error", common.GetComponentInfo())
		return
	}
	err = core.UnmarshalPrimitive(m, "updatedBy", &obj.UpdatedBy)
	if err != nil {
		err = core.SDKErrorf(err, "", "updatedBy-error", common.GetComponentInfo())
		return
	}
	err = core.UnmarshalPrimitive(m, "lastUpdated", &obj.LastUpdated)
	if err != nil {
		err = core.SDKErrorf(err, "", "lastUpdated-error", common.GetComponentInfo())
		return
	}
	err = core.UnmarshalPrimitive(m, "policy_type", &obj.PolicyType)
	if err != nil {
		err = core.SDKErrorf(err, "", "policy_type-error", common.GetComponentInfo())
		return
	}
	err = core.UnmarshalModel(m, "policy_data", &obj.PolicyData, UnmarshalGetInstancePoliciesOneOfGetInstancePolicyKeyCreateImportAccessResourcesItemPolicyData)
	if err != nil {
		err = core.SDKErrorf(err, "", "policy_data-error", common.GetComponentInfo())
		return
	}
	reflect.ValueOf(result).Elem().Set(reflect.ValueOf(obj))
	return
}

// GetInstancePoliciesOneOfGetInstancePolicyKeyCreateImportAccessResourcesItemPolicyData : User defined metadata that is associated with the `keyCreateImportAccess` instance policy type.
type GetInstancePoliciesOneOfGetInstancePolicyKeyCreateImportAccessResourcesItemPolicyData struct {
	// If set to `true`, Key Protect enables the specified policy for your service instance. If set to `false`, Key Protect
	// disables the specified policy for your service instance, and the policy will no longer affect Key Protect actions.
	// **Note:** If a policy with attributes is disabled, all attributes are reset and are not retained.
	Enabled *bool `json:"enabled" validate:"required"`

	// Data associated with the policy type `keyCreateImportAccess`.
	Attributes *GetInstancePoliciesOneOfGetInstancePolicyKeyCreateImportAccessResourcesItemPolicyDataAttributes `json:"attributes,omitempty"`
}

// UnmarshalGetInstancePoliciesOneOfGetInstancePolicyKeyCreateImportAccessResourcesItemPolicyData unmarshals an instance of GetInstancePoliciesOneOfGetInstancePolicyKeyCreateImportAccessResourcesItemPolicyData from the specified map of raw messages.
func UnmarshalGetInstancePoliciesOneOfGetInstancePolicyKeyCreateImportAccessResourcesItemPolicyData(m map[string]json.RawMessage, result interface{}) (err error) {
	obj := new(GetInstancePoliciesOneOfGetInstancePolicyKeyCreateImportAccessResourcesItemPolicyData)
	err = core.UnmarshalPrimitive(m, "enabled", &obj.Enabled)
	if err != nil {
		err = core.SDKErrorf(err, "", "enabled-error", common.GetComponentInfo())
		return
	}
	err = core.UnmarshalModel(m, "attributes", &obj.Attributes, UnmarshalGetInstancePoliciesOneOfGetInstancePolicyKeyCreateImportAccessResourcesItemPolicyDataAttributes)
	if err != nil {
		err = core.SDKErrorf(err, "", "attributes-error", common.GetComponentInfo())
		return
	}
	reflect.ValueOf(result).Elem().Set(reflect.ValueOf(obj))
	return
}

// GetInstancePoliciesOneOfGetInstancePolicyKeyCreateImportAccessResourcesItemPolicyDataAttributes : Data associated with the policy type `keyCreateImportAccess`.
type GetInstancePoliciesOneOfGetInstancePolicyKeyCreateImportAccessResourcesItemPolicyDataAttributes struct {
	// If set to `false`, the service prevents you or any authorized users from using Key Protect to create root keys in
	// the specified service instance. If set to `true`, Key Protect allows you or any authorized users to create root keys
	// in the instance.
	// **Note:** If omitted, `POST /instance/policies` will set this attribute to the default value (`true`).
	CreateRootKey *bool `json:"create_root_key" validate:"required"`

	// If set to `false`, the service prevents you or any authorized users from using Key Protect to create standard keys
	// in the specified service instance. If set to `true`, Key Protect allows you or any authorized users to create
	// standard keys in the instance.
	// **Note:** If omitted, `POST /instance/policies` will set this attribute to the default value (`true`).
	CreateStandardKey *bool `json:"create_standard_key" validate:"required"`

	// If set to `false`, the service prevents you or any authorized users from importing root keys into the specified
	// service instance. If set to `true`, Key Protect allows you or any authorized users to import root keys into the
	// instance.
	// **Note:** If omitted, `POST /instance/policies` will set this attribute to the default value (`true`).
	ImportRootKey *bool `json:"import_root_key" validate:"required"`

	// If set to `false`, the service prevents you or any authorized users from importing standard keys into the specified
	// service instance. If set to `true`, Key Protect allows you or any authorized users to import standard keys into the
	// instance.
	// **Note:** If omitted, `POST /instance/policies` will set this attribute to the default value (`true`).
	ImportStandardKey *bool `json:"import_standard_key" validate:"required"`

	// If set to `true`, the service prevents you or any authorized users from importing key material into the specified
	// service instance without using an import token. If set to `false`, Key Protect allows you or any authorized users to
	// import key material into the instance without the use of an import token.
	// **Note:** If omitted, `POST /instance/policies` will set this attribute to the default value (`false`).
	EnforceToken *bool `json:"enforce_token" validate:"required"`
}

// UnmarshalGetInstancePoliciesOneOfGetInstancePolicyKeyCreateImportAccessResourcesItemPolicyDataAttributes unmarshals an instance of GetInstancePoliciesOneOfGetInstancePolicyKeyCreateImportAccessResourcesItemPolicyDataAttributes from the specified map of raw messages.
func UnmarshalGetInstancePoliciesOneOfGetInstancePolicyKeyCreateImportAccessResourcesItemPolicyDataAttributes(m map[string]json.RawMessage, result interface{}) (err error) {
	obj := new(GetInstancePoliciesOneOfGetInstancePolicyKeyCreateImportAccessResourcesItemPolicyDataAttributes)
	err = core.UnmarshalPrimitive(m, "create_root_key", &obj.CreateRootKey)
	if err != nil {
		err = core.SDKErrorf(err, "", "create_root_key-error", common.GetComponentInfo())
		return
	}
	err = core.UnmarshalPrimitive(m, "create_standard_key", &obj.CreateStandardKey)
	if err != nil {
		err = core.SDKErrorf(err, "", "create_standard_key-error", common.GetComponentInfo())
		return
	}
	err = core.UnmarshalPrimitive(m, "import_root_key", &obj.ImportRootKey)
	if err != nil {
		err = core.SDKErrorf(err, "", "import_root_key-error", common.GetComponentInfo())
		return
	}
	err = core.UnmarshalPrimitive(m, "import_standard_key", &obj.ImportStandardKey)
	if err != nil {
		err = core.SDKErrorf(err, "", "import_standard_key-error", common.GetComponentInfo())
		return
	}
	err = core.UnmarshalPrimitive(m, "enforce_token", &obj.EnforceToken)
	if err != nil {
		err = core.SDKErrorf(err, "", "enforce_token-error", common.GetComponentInfo())
		return
	}
	reflect.ValueOf(result).Elem().Set(reflect.ValueOf(obj))
	return
}

// GetInstancePoliciesOneOfResourcesItem : GetInstancePoliciesOneOfResourcesItem struct
type GetInstancePoliciesOneOfResourcesItem struct {
	// The date the policy was created. The date format follows RFC 3339.
	CreationDate *strfmt.DateTime `json:"creationDate,omitempty"`

	// The unique identifier for the resource that created the policy.
	CreatedBy *string `json:"createdBy,omitempty"`

	// The unique identifier for the resource that updated the policy.
	UpdatedBy *string `json:"updatedBy,omitempty"`

	// Updates when the policy is replaced or modified. The date format follows RFC 3339.
	LastUpdated *strfmt.DateTime `json:"lastUpdated,omitempty"`

	// The type of policy to be retrieved.
	PolicyType *string `json:"policy_type" validate:"required"`

	// User defined metadata that is associated with the `allowedNetwork` instance policy type.
	PolicyData *GetInstancePoliciesOneOfResourcesItemPolicyData `json:"policy_data" validate:"required"`
}

// UnmarshalGetInstancePoliciesOneOfResourcesItem unmarshals an instance of GetInstancePoliciesOneOfResourcesItem from the specified map of raw messages.
func UnmarshalGetInstancePoliciesOneOfResourcesItem(m map[string]json.RawMessage, result interface{}) (err error) {
	obj := new(GetInstancePoliciesOneOfResourcesItem)
	err = core.UnmarshalPrimitive(m, "creationDate", &obj.CreationDate)
	if err != nil {
		err = core.SDKErrorf(err, "", "creationDate-error", common.GetComponentInfo())
		return
	}
	err = core.UnmarshalPrimitive(m, "createdBy", &obj.CreatedBy)
	if err != nil {
		err = core.SDKErrorf(err, "", "createdBy-error", common.GetComponentInfo())
		return
	}
	err = core.UnmarshalPrimitive(m, "updatedBy", &obj.UpdatedBy)
	if err != nil {
		err = core.SDKErrorf(err, "", "updatedBy-error", common.GetComponentInfo())
		return
	}
	err = core.UnmarshalPrimitive(m, "lastUpdated", &obj.LastUpdated)
	if err != nil {
		err = core.SDKErrorf(err, "", "lastUpdated-error", common.GetComponentInfo())
		return
	}
	err = core.UnmarshalPrimitive(m, "policy_type", &obj.PolicyType)
	if err != nil {
		err = core.SDKErrorf(err, "", "policy_type-error", common.GetComponentInfo())
		return
	}
	err = core.UnmarshalModel(m, "policy_data", &obj.PolicyData, UnmarshalGetInstancePoliciesOneOfResourcesItemPolicyData)
	if err != nil {
		err = core.SDKErrorf(err, "", "policy_data-error", common.GetComponentInfo())
		return
	}
	reflect.ValueOf(result).Elem().Set(reflect.ValueOf(obj))
	return
}

// GetInstancePoliciesOneOfResourcesItemPolicyData : User defined metadata that is associated with the `allowedNetwork` instance policy type.
type GetInstancePoliciesOneOfResourcesItemPolicyData struct {
	// If set to `true`, Key Protect enables the specified policy for your service instance. If set to `false`, Key Protect
	// disables the specified policy for your service instance, and the policy will no longer affect Key Protect actions.
	// **Note:** If a policy with attributes is disabled, all attributes are reset and are not retained.
	Enabled *bool `json:"enabled" validate:"required"`

	// Data associated with the policy type `allowed_network`.
	Attributes *GetInstancePoliciesOneOfResourcesItemPolicyDataAttributes `json:"attributes,omitempty"`
}

// UnmarshalGetInstancePoliciesOneOfResourcesItemPolicyData unmarshals an instance of GetInstancePoliciesOneOfResourcesItemPolicyData from the specified map of raw messages.
func UnmarshalGetInstancePoliciesOneOfResourcesItemPolicyData(m map[string]json.RawMessage, result interface{}) (err error) {
	obj := new(GetInstancePoliciesOneOfResourcesItemPolicyData)
	err = core.UnmarshalPrimitive(m, "enabled", &obj.Enabled)
	if err != nil {
		err = core.SDKErrorf(err, "", "enabled-error", common.GetComponentInfo())
		return
	}
	err = core.UnmarshalModel(m, "attributes", &obj.Attributes, UnmarshalGetInstancePoliciesOneOfResourcesItemPolicyDataAttributes)
	if err != nil {
		err = core.SDKErrorf(err, "", "attributes-error", common.GetComponentInfo())
		return
	}
	reflect.ValueOf(result).Elem().Set(reflect.ValueOf(obj))
	return
}

// GetInstancePoliciesOneOfResourcesItemPolicyDataAttributes : Data associated with the policy type `allowed_network`.
type GetInstancePoliciesOneOfResourcesItemPolicyDataAttributes struct {
	// If set to `public-and-private`, Key Protect allows the instance to be accessible through public and private
	// endpoints. If set to `private-only`, Key Protect restricts the instance to only be accessible through a private
	// endpoint.
	AllowedNetwork *string `json:"allowed_network" validate:"required"`
}

// Constants associated with the GetInstancePoliciesOneOfResourcesItemPolicyDataAttributes.AllowedNetwork property.
// If set to `public-and-private`, Key Protect allows the instance to be accessible through public and private
// endpoints. If set to `private-only`, Key Protect restricts the instance to only be accessible through a private
// endpoint.
const (
	GetInstancePoliciesOneOfResourcesItemPolicyDataAttributes_AllowedNetwork_PrivateOnly      = "private-only"
	GetInstancePoliciesOneOfResourcesItemPolicyDataAttributes_AllowedNetwork_PublicAndPrivate = "public-and-private"
)

// UnmarshalGetInstancePoliciesOneOfResourcesItemPolicyDataAttributes unmarshals an instance of GetInstancePoliciesOneOfResourcesItemPolicyDataAttributes from the specified map of raw messages.
func UnmarshalGetInstancePoliciesOneOfResourcesItemPolicyDataAttributes(m map[string]json.RawMessage, result interface{}) (err error) {
	obj := new(GetInstancePoliciesOneOfResourcesItemPolicyDataAttributes)
	err = core.UnmarshalPrimitive(m, "allowed_network", &obj.AllowedNetwork)
	if err != nil {
		err = core.SDKErrorf(err, "", "allowed_network-error", common.GetComponentInfo())
		return
	}
	reflect.ValueOf(result).Elem().Set(reflect.ValueOf(obj))
	return
}

// GetInstancePolicyAllowedIPResourcesItem : GetInstancePolicyAllowedIPResourcesItem struct
type GetInstancePolicyAllowedIPResourcesItem struct {
	// The date the policy was created. The date format follows RFC 3339.
	CreationDate *strfmt.DateTime `json:"creationDate,omitempty"`

	// The unique identifier for the resource that created the policy.
	CreatedBy *string `json:"createdBy,omitempty"`

	// The unique identifier for the resource that updated the policy.
	UpdatedBy *string `json:"updatedBy,omitempty"`

	// Updates when the policy is replaced or modified. The date format follows RFC 3339.
	LastUpdated *strfmt.DateTime `json:"lastUpdated,omitempty"`

	// The type of policy to be retrieved.
	PolicyType *string `json:"policy_type" validate:"required"`

	// User defined metadata that is associated with the `allowedIP` instance policy type.
	PolicyData *GetInstancePolicyAllowedIPResourcesItemPolicyData `json:"policy_data" validate:"required"`
}

// UnmarshalGetInstancePolicyAllowedIPResourcesItem unmarshals an instance of GetInstancePolicyAllowedIPResourcesItem from the specified map of raw messages.
func UnmarshalGetInstancePolicyAllowedIPResourcesItem(m map[string]json.RawMessage, result interface{}) (err error) {
	obj := new(GetInstancePolicyAllowedIPResourcesItem)
	err = core.UnmarshalPrimitive(m, "creationDate", &obj.CreationDate)
	if err != nil {
		err = core.SDKErrorf(err, "", "creationDate-error", common.GetComponentInfo())
		return
	}
	err = core.UnmarshalPrimitive(m, "createdBy", &obj.CreatedBy)
	if err != nil {
		err = core.SDKErrorf(err, "", "createdBy-error", common.GetComponentInfo())
		return
	}
	err = core.UnmarshalPrimitive(m, "updatedBy", &obj.UpdatedBy)
	if err != nil {
		err = core.SDKErrorf(err, "", "updatedBy-error", common.GetComponentInfo())
		return
	}
	err = core.UnmarshalPrimitive(m, "lastUpdated", &obj.LastUpdated)
	if err != nil {
		err = core.SDKErrorf(err, "", "lastUpdated-error", common.GetComponentInfo())
		return
	}
	err = core.UnmarshalPrimitive(m, "policy_type", &obj.PolicyType)
	if err != nil {
		err = core.SDKErrorf(err, "", "policy_type-error", common.GetComponentInfo())
		return
	}
	err = core.UnmarshalModel(m, "policy_data", &obj.PolicyData, UnmarshalGetInstancePolicyAllowedIPResourcesItemPolicyData)
	if err != nil {
		err = core.SDKErrorf(err, "", "policy_data-error", common.GetComponentInfo())
		return
	}
	reflect.ValueOf(result).Elem().Set(reflect.ValueOf(obj))
	return
}

// GetInstancePolicyAllowedIPResourcesItemPolicyData : User defined metadata that is associated with the `allowedIP` instance policy type.
type GetInstancePolicyAllowedIPResourcesItemPolicyData struct {
	// If set to `true`, Key Protect enables the specified policy for your service instance. If set to `false`, Key Protect
	// disables the specified policy for your service instance, and the policy will no longer affect Key Protect actions.
	// **Note:** If a policy with attributes is disabled, all attributes are reset and are not retained.
	Enabled *bool `json:"enabled" validate:"required"`

	// Data associated with the policy type `allowedIP`.
	Attributes *GetInstancePolicyAllowedIPResourcesItemPolicyDataAttributes `json:"attributes,omitempty"`
}

// UnmarshalGetInstancePolicyAllowedIPResourcesItemPolicyData unmarshals an instance of GetInstancePolicyAllowedIPResourcesItemPolicyData from the specified map of raw messages.
func UnmarshalGetInstancePolicyAllowedIPResourcesItemPolicyData(m map[string]json.RawMessage, result interface{}) (err error) {
	obj := new(GetInstancePolicyAllowedIPResourcesItemPolicyData)
	err = core.UnmarshalPrimitive(m, "enabled", &obj.Enabled)
	if err != nil {
		err = core.SDKErrorf(err, "", "enabled-error", common.GetComponentInfo())
		return
	}
	err = core.UnmarshalModel(m, "attributes", &obj.Attributes, UnmarshalGetInstancePolicyAllowedIPResourcesItemPolicyDataAttributes)
	if err != nil {
		err = core.SDKErrorf(err, "", "attributes-error", common.GetComponentInfo())
		return
	}
	reflect.ValueOf(result).Elem().Set(reflect.ValueOf(obj))
	return
}

// GetInstancePolicyAllowedIPResourcesItemPolicyDataAttributes : Data associated with the policy type `allowedIP`.
type GetInstancePolicyAllowedIPResourcesItemPolicyDataAttributes struct {
	// A string array of IPv4 or IPv6 CIDR notated subnets that are authorized to interact with the instance. If both
	// `allowedNetwork` and `allowedIP` policies are set, only traffic aligning with both the `allowed_network` allowed
	// network policy attribute and the `allowed_ip` allowed IP policy attribute will be allowed. IPv4 and iIP6 addresses
	// are accepted for public endpoints. Only the IPv4 private network gateway addresses from the array will be authorized
	// to access your instance via private endpoint.
	// **Important:** Once set, accessing your instance may require additional steps. For more information, see [Accessing
	// an instance via public
	// endpoint](/docs/key-protect?topic=key-protect-manage-allowed-ip#access-allowed-ip-public-endpoint) and [Accessing an
	// instance via private
	// endpoint](/docs/key-protect?topic=key-protect-manage-allowed-ip#access-allowed-ip-private-endpoint) for more
	// details.
	// **Note:** An allowed IP policy does not affect requests from other IBM Cloud services.
	AllowedIp []string `json:"allowed_ip,omitempty"`
}

// UnmarshalGetInstancePolicyAllowedIPResourcesItemPolicyDataAttributes unmarshals an instance of GetInstancePolicyAllowedIPResourcesItemPolicyDataAttributes from the specified map of raw messages.
func UnmarshalGetInstancePolicyAllowedIPResourcesItemPolicyDataAttributes(m map[string]json.RawMessage, result interface{}) (err error) {
	obj := new(GetInstancePolicyAllowedIPResourcesItemPolicyDataAttributes)
	err = core.UnmarshalPrimitive(m, "allowed_ip", &obj.AllowedIp)
	if err != nil {
		err = core.SDKErrorf(err, "", "allowed_ip-error", common.GetComponentInfo())
		return
	}
	reflect.ValueOf(result).Elem().Set(reflect.ValueOf(obj))
	return
}

// GetInstancePolicyDualAuthDeleteResourcesItem : GetInstancePolicyDualAuthDeleteResourcesItem struct
type GetInstancePolicyDualAuthDeleteResourcesItem struct {
	// The date the policy was created. The date format follows RFC 3339.
	CreationDate *strfmt.DateTime `json:"creationDate,omitempty"`

	// The unique identifier for the resource that created the policy.
	CreatedBy *string `json:"createdBy,omitempty"`

	// The unique identifier for the resource that updated the policy.
	UpdatedBy *string `json:"updatedBy,omitempty"`

	// Updates when the policy is replaced or modified. The date format follows RFC 3339.
	LastUpdated *strfmt.DateTime `json:"lastUpdated,omitempty"`

	// The type of policy to be retrieved.
	PolicyType *string `json:"policy_type" validate:"required"`

	// User defined metadata that is associated with the `dualAuthDelete` instance policy type.
	PolicyData *DualAuthDeleteProperties `json:"policy_data" validate:"required"`
}

// UnmarshalGetInstancePolicyDualAuthDeleteResourcesItem unmarshals an instance of GetInstancePolicyDualAuthDeleteResourcesItem from the specified map of raw messages.
func UnmarshalGetInstancePolicyDualAuthDeleteResourcesItem(m map[string]json.RawMessage, result interface{}) (err error) {
	obj := new(GetInstancePolicyDualAuthDeleteResourcesItem)
	err = core.UnmarshalPrimitive(m, "creationDate", &obj.CreationDate)
	if err != nil {
		err = core.SDKErrorf(err, "", "creationDate-error", common.GetComponentInfo())
		return
	}
	err = core.UnmarshalPrimitive(m, "createdBy", &obj.CreatedBy)
	if err != nil {
		err = core.SDKErrorf(err, "", "createdBy-error", common.GetComponentInfo())
		return
	}
	err = core.UnmarshalPrimitive(m, "updatedBy", &obj.UpdatedBy)
	if err != nil {
		err = core.SDKErrorf(err, "", "updatedBy-error", common.GetComponentInfo())
		return
	}
	err = core.UnmarshalPrimitive(m, "lastUpdated", &obj.LastUpdated)
	if err != nil {
		err = core.SDKErrorf(err, "", "lastUpdated-error", common.GetComponentInfo())
		return
	}
	err = core.UnmarshalPrimitive(m, "policy_type", &obj.PolicyType)
	if err != nil {
		err = core.SDKErrorf(err, "", "policy_type-error", common.GetComponentInfo())
		return
	}
	err = core.UnmarshalModel(m, "policy_data", &obj.PolicyData, UnmarshalDualAuthDeleteProperties)
	if err != nil {
		err = core.SDKErrorf(err, "", "policy_data-error", common.GetComponentInfo())
		return
	}
	reflect.ValueOf(result).Elem().Set(reflect.ValueOf(obj))
	return
}

// GetInstancePolicyMetricsResourcesItem : GetInstancePolicyMetricsResourcesItem struct
type GetInstancePolicyMetricsResourcesItem struct {
	// The date the policy was created. The date format follows RFC 3339.
	CreationDate *strfmt.DateTime `json:"creationDate,omitempty"`

	// The unique identifier for the resource that created the policy.
	CreatedBy *string `json:"createdBy,omitempty"`

	// The unique identifier for the resource that updated the policy.
	UpdatedBy *string `json:"updatedBy,omitempty"`

	// Updates when the policy is replaced or modified. The date format follows RFC 3339.
	LastUpdated *strfmt.DateTime `json:"lastUpdated,omitempty"`

	// The type of policy to be retrieved.
	PolicyType *string `json:"policy_type" validate:"required"`

	// User defined metadata that is associated with the `metrics` instance policy type.
	PolicyData *MetricsProperties `json:"policy_data" validate:"required"`
}

// UnmarshalGetInstancePolicyMetricsResourcesItem unmarshals an instance of GetInstancePolicyMetricsResourcesItem from the specified map of raw messages.
func UnmarshalGetInstancePolicyMetricsResourcesItem(m map[string]json.RawMessage, result interface{}) (err error) {
	obj := new(GetInstancePolicyMetricsResourcesItem)
	err = core.UnmarshalPrimitive(m, "creationDate", &obj.CreationDate)
	if err != nil {
		err = core.SDKErrorf(err, "", "creationDate-error", common.GetComponentInfo())
		return
	}
	err = core.UnmarshalPrimitive(m, "createdBy", &obj.CreatedBy)
	if err != nil {
		err = core.SDKErrorf(err, "", "createdBy-error", common.GetComponentInfo())
		return
	}
	err = core.UnmarshalPrimitive(m, "updatedBy", &obj.UpdatedBy)
	if err != nil {
		err = core.SDKErrorf(err, "", "updatedBy-error", common.GetComponentInfo())
		return
	}
	err = core.UnmarshalPrimitive(m, "lastUpdated", &obj.LastUpdated)
	if err != nil {
		err = core.SDKErrorf(err, "", "lastUpdated-error", common.GetComponentInfo())
		return
	}
	err = core.UnmarshalPrimitive(m, "policy_type", &obj.PolicyType)
	if err != nil {
		err = core.SDKErrorf(err, "", "policy_type-error", common.GetComponentInfo())
		return
	}
	err = core.UnmarshalModel(m, "policy_data", &obj.PolicyData, UnmarshalMetricsProperties)
	if err != nil {
		err = core.SDKErrorf(err, "", "policy_data-error", common.GetComponentInfo())
		return
	}
	reflect.ValueOf(result).Elem().Set(reflect.ValueOf(obj))
	return
}

// GetInstancePolicyOptions : The GetInstancePolicy options.
type GetInstancePolicyOptions struct {
	// The IBM Cloud instance ID that identifies your Key Protect service instance.
	BluemixInstance *string `json:"Bluemix-Instance" validate:"required"`

	// The v4 UUID used to correlate and track transactions.
	CorrelationID *string `json:"Correlation-Id,omitempty"`

	// The type of policy that is associated with the specified instance.
	Policy *string `json:"policy,omitempty"`

	// Allows users to set headers on API requests.
	Headers map[string]string
}

// Constants associated with the GetInstancePolicyOptions.Policy property.
// The type of policy that is associated with the specified instance.
const (
	GetInstancePolicyOptions_Policy_Allowedip             = "allowedIP"
	GetInstancePolicyOptions_Policy_Allowednetwork        = "allowedNetwork"
	GetInstancePolicyOptions_Policy_Dualauthdelete        = "dualAuthDelete"
	GetInstancePolicyOptions_Policy_Keycreateimportaccess = "keyCreateImportAccess"
	GetInstancePolicyOptions_Policy_Metrics               = "metrics"
	GetInstancePolicyOptions_Policy_Rotation              = "rotation"
)

// NewGetInstancePolicyOptions : Instantiate GetInstancePolicyOptions
func (*IbmKeyProtectApiV2) NewGetInstancePolicyOptions(bluemixInstance string) *GetInstancePolicyOptions {
	return &GetInstancePolicyOptions{
		BluemixInstance: core.StringPtr(bluemixInstance),
	}
}

// SetBluemixInstance : Allow user to set BluemixInstance
func (_options *GetInstancePolicyOptions) SetBluemixInstance(bluemixInstance string) *GetInstancePolicyOptions {
	_options.BluemixInstance = core.StringPtr(bluemixInstance)
	return _options
}

// SetCorrelationID : Allow user to set CorrelationID
func (_options *GetInstancePolicyOptions) SetCorrelationID(correlationID string) *GetInstancePolicyOptions {
	_options.CorrelationID = core.StringPtr(correlationID)
	return _options
}

// SetPolicy : Allow user to set Policy
func (_options *GetInstancePolicyOptions) SetPolicy(policy string) *GetInstancePolicyOptions {
	_options.Policy = core.StringPtr(policy)
	return _options
}

// SetHeaders : Allow user to set Headers
func (options *GetInstancePolicyOptions) SetHeaders(param map[string]string) *GetInstancePolicyOptions {
	options.Headers = param
	return options
}

// GetInstancePolicyRotationResourcesItem : GetInstancePolicyRotationResourcesItem struct
type GetInstancePolicyRotationResourcesItem struct {
	// The date the policy was created. The date format follows RFC 3339.
	CreationDate *strfmt.DateTime `json:"creationDate,omitempty"`

	// The unique identifier for the resource that created the policy.
	CreatedBy *string `json:"createdBy,omitempty"`

	// The unique identifier for the resource that updated the policy.
	UpdatedBy *string `json:"updatedBy,omitempty"`

	// Updates when the policy is replaced or modified. The date format follows RFC 3339.
	LastUpdated *strfmt.DateTime `json:"lastUpdated,omitempty"`

	// The type of policy to be retrieved.
	PolicyType *string `json:"policy_type" validate:"required"`

	// User defined metadata that is associated with the `rotation` instance policy type.
	PolicyData *GetInstancePolicyRotationResourcesItemPolicyData `json:"policy_data" validate:"required"`
}

// UnmarshalGetInstancePolicyRotationResourcesItem unmarshals an instance of GetInstancePolicyRotationResourcesItem from the specified map of raw messages.
func UnmarshalGetInstancePolicyRotationResourcesItem(m map[string]json.RawMessage, result interface{}) (err error) {
	obj := new(GetInstancePolicyRotationResourcesItem)
	err = core.UnmarshalPrimitive(m, "creationDate", &obj.CreationDate)
	if err != nil {
		err = core.SDKErrorf(err, "", "creationDate-error", common.GetComponentInfo())
		return
	}
	err = core.UnmarshalPrimitive(m, "createdBy", &obj.CreatedBy)
	if err != nil {
		err = core.SDKErrorf(err, "", "createdBy-error", common.GetComponentInfo())
		return
	}
	err = core.UnmarshalPrimitive(m, "updatedBy", &obj.UpdatedBy)
	if err != nil {
		err = core.SDKErrorf(err, "", "updatedBy-error", common.GetComponentInfo())
		return
	}
	err = core.UnmarshalPrimitive(m, "lastUpdated", &obj.LastUpdated)
	if err != nil {
		err = core.SDKErrorf(err, "", "lastUpdated-error", common.GetComponentInfo())
		return
	}
	err = core.UnmarshalPrimitive(m, "policy_type", &obj.PolicyType)
	if err != nil {
		err = core.SDKErrorf(err, "", "policy_type-error", common.GetComponentInfo())
		return
	}
	err = core.UnmarshalModel(m, "policy_data", &obj.PolicyData, UnmarshalGetInstancePolicyRotationResourcesItemPolicyData)
	if err != nil {
		err = core.SDKErrorf(err, "", "policy_data-error", common.GetComponentInfo())
		return
	}
	reflect.ValueOf(result).Elem().Set(reflect.ValueOf(obj))
	return
}

// GetInstancePolicyRotationResourcesItemPolicyData : User defined metadata that is associated with the `rotation` instance policy type.
type GetInstancePolicyRotationResourcesItemPolicyData struct {
	// If set to `true`, Key Protect enables the specified policy for your service instance. If set to `false`, Key Protect
	// disables the specified policy for your service instance, and the policy will no longer affect Key Protect actions.
	// **Note:** If a policy with attributes is disabled, all attributes are reset and are not retained.
	Enabled *bool `json:"enabled" validate:"required"`

	// Data associated with the policy type `rotation`.
	Attributes *GetInstancePolicyRotationResourcesItemPolicyDataAttributes `json:"attributes,omitempty"`
}

// UnmarshalGetInstancePolicyRotationResourcesItemPolicyData unmarshals an instance of GetInstancePolicyRotationResourcesItemPolicyData from the specified map of raw messages.
func UnmarshalGetInstancePolicyRotationResourcesItemPolicyData(m map[string]json.RawMessage, result interface{}) (err error) {
	obj := new(GetInstancePolicyRotationResourcesItemPolicyData)
	err = core.UnmarshalPrimitive(m, "enabled", &obj.Enabled)
	if err != nil {
		err = core.SDKErrorf(err, "", "enabled-error", common.GetComponentInfo())
		return
	}
	err = core.UnmarshalModel(m, "attributes", &obj.Attributes, UnmarshalGetInstancePolicyRotationResourcesItemPolicyDataAttributes)
	if err != nil {
		err = core.SDKErrorf(err, "", "attributes-error", common.GetComponentInfo())
		return
	}
	reflect.ValueOf(result).Elem().Set(reflect.ValueOf(obj))
	return
}

// GetInstancePolicyRotationResourcesItemPolicyDataAttributes : Data associated with the policy type `rotation`.
type GetInstancePolicyRotationResourcesItemPolicyDataAttributes struct {
	// Specifies the key rotation time interval in approximate months, where a month is equivalent to 30 days. A minimum of
	// 1 and a maximum of 12 can be set.
	IntervalMonth *int64 `json:"interval_month" validate:"required"`
}

// UnmarshalGetInstancePolicyRotationResourcesItemPolicyDataAttributes unmarshals an instance of GetInstancePolicyRotationResourcesItemPolicyDataAttributes from the specified map of raw messages.
func UnmarshalGetInstancePolicyRotationResourcesItemPolicyDataAttributes(m map[string]json.RawMessage, result interface{}) (err error) {
	obj := new(GetInstancePolicyRotationResourcesItemPolicyDataAttributes)
	err = core.UnmarshalPrimitive(m, "interval_month", &obj.IntervalMonth)
	if err != nil {
		err = core.SDKErrorf(err, "", "interval_month-error", common.GetComponentInfo())
		return
	}
	reflect.ValueOf(result).Elem().Set(reflect.ValueOf(obj))
	return
}

// GetKey : The base schema for retrieving keys.
type GetKey struct {
	// The metadata that describes the resource array.
	Metadata *CollectionMetadata `json:"metadata" validate:"required"`

	// A collection of resources.
	Resources []KeyWithPayload `json:"resources" validate:"required"`
}

// UnmarshalGetKey unmarshals an instance of GetKey from the specified map of raw messages.
func UnmarshalGetKey(m map[string]json.RawMessage, result interface{}) (err error) {
	obj := new(GetKey)
	err = core.UnmarshalModel(m, "metadata", &obj.Metadata, UnmarshalCollectionMetadata)
	if err != nil {
		err = core.SDKErrorf(err, "", "metadata-error", common.GetComponentInfo())
		return
	}
	err = core.UnmarshalModel(m, "resources", &obj.Resources, UnmarshalKeyWithPayload)
	if err != nil {
		err = core.SDKErrorf(err, "", "resources-error", common.GetComponentInfo())
		return
	}
	reflect.ValueOf(result).Elem().Set(reflect.ValueOf(obj))
	return
}

// GetKeyCollectionMetadataOptions : The GetKeyCollectionMetadata options.
type GetKeyCollectionMetadataOptions struct {
	// The IBM Cloud instance ID that identifies your Key Protect service instance.
	BluemixInstance *string `json:"Bluemix-Instance" validate:"required"`

	// The v4 UUID used to correlate and track transactions.
	CorrelationID *string `json:"Correlation-Id,omitempty"`

	// The state of the keys to be retrieved. States must be a list of integers from 0 to 5 delimited by commas with no
	// whitespace or trailing commas. Valid states are based on NIST SP 800-57. States are integers and correspond to the
	// Pre-activation = 0, Active = 1, Suspended = 2, Deactivated = 3, and Destroyed = 5 values.
	// **Usage:** If you want to retrieve active and deleted keys, use `../keys?state=1,5`.
	State []int64 `json:"state,omitempty"`

	// The type of keys to be retrieved. Filters keys based on the `extractable` property. You can use this query parameter
	// to search for keys whose material can leave the service. If set to `true`, standard keys will be retrieved. If set
	// to `false`, root keys will be retrieved. If omitted, both root and standard keys will be retrieved.
	// **Usage:** If you want to retrieve standard keys, use `../keys?extractable=true`.
	Extractable *bool `json:"extractable,omitempty"`

	// When provided, returns the list of keys that match the queried properties. Each key property to be filtered on is
	// specified as the property name itself, followed by an “=“ symbol, and then the value to filter on, followed by a
	// space if there are more properties to filter only. Note: Anything between `<` and `>` in the examples or
	// descriptions represent placeholder to specify the value
	// *Basic format*: <propertyA>=<valueB> <propertyB>=<valueB> - The value to filter on may contain a value related to
	// the property itself, or an operator followed by a value accepted by the operator - Only one operator and value, or
	// one value is accepted per property at a time
	// *Format with operator/value pair*: <propertyA>=<operatorA>:<valueA> Up to three of the same property may be
	// specified at a time. The key properties that can be filtered at this time are:
	// - `creationDate`
	//   * Date in RFC 3339 format in double-quotes: “2000-03-21T00:00:00Z”
	// - `deletionDate`
	//   * Date in RFC 3339 format in double-quotes: “2000-03-21T00:00:00Z”
	// - `expirationDate`
	//   * Date in RFC 3339 format in double-quotes: “2000-03-21T00:00:00Z”
	// - `extractable`
	//   * Boolean true or false without quotes, case-insensitive
	// - `lastRotateDate`
	//   * Date in RFC 3339 format in double-quotes: “2000-03-21T00:00:00Z”
	// - `lastUpdateDate`
	//   * Date in RFC 3339 format in double-quotes: “2000-03-21T00:00:00Z”
	// - `state`
	//   * A list of comma-separated integers with no space in between: 0,1,2,3,5 Comparison operations (operators) that
	// can be performed on date values are:
	// - `lte:<value>` Less than or equal to - `lt:<value>` Less than - `gte:<value>` Greater than or equal to -
	// `gt:<value>` Greater than A special keyword for date, `none` (case-insensitive), may be used to retreive keys that
	// do not have that property. This is useful for `lastRotateDate`, where only keys that have never been rotated can be
	// retreived.
	// *Examples*:
	// - `lastRotateDate="2022-02-15T00:00:00Z"` Filter keys that were last rotated on February 15, 2022 -
	// `lastRotateDate=gte:"2022-02-15T00:00:00Z"` Filter keys that were last rotated after or on February 15, 2022 -
	// `lastRotateDate=gte:"2022-02-15T00:00:00Z" lastRotateDate=lt:"2022-03-15T00:00:00Z"` Filter keys that were last
	// rotated after or on February 15, 2022 but before (not including) March 15, 2022 -
	// `lastRotateDate="2022-02-15T00:00:00Z" state=0,1,2,3,5 extractable=false` Filter root keys that were last rotated on
	// February 15, 2022, with any state
	// *Note*: When you filter by `state` or `extractable` in this query parameter, you will not be able to use the
	// deprecated `state` or `extractable` independent query parameter. You will get a 400 response code if you specify a
	// value for one of the two properties in both this filter query parameter and the deprecated independent query of the
	// same name (the same applies vice versa).
	Filter *string `json:"filter,omitempty"`

	// The ID of the target key ring. If unspecified, all resources in the instance that the caller has access to will be
	// returned. When the header is specified, only resources within the specified key ring, that the caller has access to,
	// will be returned. The key ring ID of keys that are created without an `X-Kms-Key-Ring` header is: `default`.
	XKmsKeyRing *string `json:"X-Kms-Key-Ring,omitempty"`

	// Allows users to set headers on API requests.
	Headers map[string]string
}

// NewGetKeyCollectionMetadataOptions : Instantiate GetKeyCollectionMetadataOptions
func (*IbmKeyProtectApiV2) NewGetKeyCollectionMetadataOptions(bluemixInstance string) *GetKeyCollectionMetadataOptions {
	return &GetKeyCollectionMetadataOptions{
		BluemixInstance: core.StringPtr(bluemixInstance),
	}
}

// SetBluemixInstance : Allow user to set BluemixInstance
func (_options *GetKeyCollectionMetadataOptions) SetBluemixInstance(bluemixInstance string) *GetKeyCollectionMetadataOptions {
	_options.BluemixInstance = core.StringPtr(bluemixInstance)
	return _options
}

// SetCorrelationID : Allow user to set CorrelationID
func (_options *GetKeyCollectionMetadataOptions) SetCorrelationID(correlationID string) *GetKeyCollectionMetadataOptions {
	_options.CorrelationID = core.StringPtr(correlationID)
	return _options
}

// SetState : Allow user to set State
func (_options *GetKeyCollectionMetadataOptions) SetState(state []int64) *GetKeyCollectionMetadataOptions {
	_options.State = state
	return _options
}

// SetExtractable : Allow user to set Extractable
func (_options *GetKeyCollectionMetadataOptions) SetExtractable(extractable bool) *GetKeyCollectionMetadataOptions {
	_options.Extractable = core.BoolPtr(extractable)
	return _options
}

// SetFilter : Allow user to set Filter
func (_options *GetKeyCollectionMetadataOptions) SetFilter(filter string) *GetKeyCollectionMetadataOptions {
	_options.Filter = core.StringPtr(filter)
	return _options
}

// SetXKmsKeyRing : Allow user to set XKmsKeyRing
func (_options *GetKeyCollectionMetadataOptions) SetXKmsKeyRing(xKmsKeyRing string) *GetKeyCollectionMetadataOptions {
	_options.XKmsKeyRing = core.StringPtr(xKmsKeyRing)
	return _options
}

// SetHeaders : Allow user to set Headers
func (options *GetKeyCollectionMetadataOptions) SetHeaders(param map[string]string) *GetKeyCollectionMetadataOptions {
	options.Headers = param
	return options
}

// GetKeyMetadata : The base schema for retrieving key metadata.
type GetKeyMetadata struct {
	// The metadata that describes the resource array.
	Metadata *CollectionMetadata `json:"metadata" validate:"required"`

	// A collection of resources.
	Resources []KeyFullRepresentation `json:"resources" validate:"required"`
}

// UnmarshalGetKeyMetadata unmarshals an instance of GetKeyMetadata from the specified map of raw messages.
func UnmarshalGetKeyMetadata(m map[string]json.RawMessage, result interface{}) (err error) {
	obj := new(GetKeyMetadata)
	err = core.UnmarshalModel(m, "metadata", &obj.Metadata, UnmarshalCollectionMetadata)
	if err != nil {
		err = core.SDKErrorf(err, "", "metadata-error", common.GetComponentInfo())
		return
	}
	err = core.UnmarshalModel(m, "resources", &obj.Resources, UnmarshalKeyFullRepresentation)
	if err != nil {
		err = core.SDKErrorf(err, "", "resources-error", common.GetComponentInfo())
		return
	}
	reflect.ValueOf(result).Elem().Set(reflect.ValueOf(obj))
	return
}

// GetKeyMetadataOptions : The GetKeyMetadata options.
type GetKeyMetadataOptions struct {
	// The v4 UUID or alias that uniquely identifies the key.
	ID *string `json:"id" validate:"required,ne="`

	// The IBM Cloud instance ID that identifies your Key Protect service instance.
	BluemixInstance *string `json:"Bluemix-Instance" validate:"required"`

	// The v4 UUID used to correlate and track transactions.
	CorrelationID *string `json:"Correlation-Id,omitempty"`

	// The ID of the key ring that the specified key is a part of. When the header is not specified, Key Protect will
	// perform a key ring lookup. For a more optimized request, specify the key ring on every call. The key ring ID of keys
	// that are created without an `X-Kms-Key-Ring` header is: `default`.
	XKmsKeyRing *string `json:"X-Kms-Key-Ring,omitempty"`

	// Allows users to set headers on API requests.
	Headers map[string]string
}

// NewGetKeyMetadataOptions : Instantiate GetKeyMetadataOptions
func (*IbmKeyProtectApiV2) NewGetKeyMetadataOptions(id string, bluemixInstance string) *GetKeyMetadataOptions {
	return &GetKeyMetadataOptions{
		ID:              core.StringPtr(id),
		BluemixInstance: core.StringPtr(bluemixInstance),
	}
}

// SetID : Allow user to set ID
func (_options *GetKeyMetadataOptions) SetID(id string) *GetKeyMetadataOptions {
	_options.ID = core.StringPtr(id)
	return _options
}

// SetBluemixInstance : Allow user to set BluemixInstance
func (_options *GetKeyMetadataOptions) SetBluemixInstance(bluemixInstance string) *GetKeyMetadataOptions {
	_options.BluemixInstance = core.StringPtr(bluemixInstance)
	return _options
}

// SetCorrelationID : Allow user to set CorrelationID
func (_options *GetKeyMetadataOptions) SetCorrelationID(correlationID string) *GetKeyMetadataOptions {
	_options.CorrelationID = core.StringPtr(correlationID)
	return _options
}

// SetXKmsKeyRing : Allow user to set XKmsKeyRing
func (_options *GetKeyMetadataOptions) SetXKmsKeyRing(xKmsKeyRing string) *GetKeyMetadataOptions {
	_options.XKmsKeyRing = core.StringPtr(xKmsKeyRing)
	return _options
}

// SetHeaders : Allow user to set Headers
func (options *GetKeyMetadataOptions) SetHeaders(param map[string]string) *GetKeyMetadataOptions {
	options.Headers = param
	return options
}

// GetKeyOptions : The GetKey options.
type GetKeyOptions struct {
	// The v4 UUID or alias that uniquely identifies the key.
	ID *string `json:"id" validate:"required,ne="`

	// The IBM Cloud instance ID that identifies your Key Protect service instance.
	BluemixInstance *string `json:"Bluemix-Instance" validate:"required"`

	// The v4 UUID used to correlate and track transactions.
	CorrelationID *string `json:"Correlation-Id,omitempty"`

	// The ID of the key ring that the specified key is a part of. When the header is not specified, Key Protect will
	// perform a key ring lookup. For a more optimized request, specify the key ring on every call. The key ring ID of keys
	// that are created without an `X-Kms-Key-Ring` header is: `default`.
	XKmsKeyRing *string `json:"X-Kms-Key-Ring,omitempty"`

	// Allows users to set headers on API requests.
	Headers map[string]string
}

// NewGetKeyOptions : Instantiate GetKeyOptions
func (*IbmKeyProtectApiV2) NewGetKeyOptions(id string, bluemixInstance string) *GetKeyOptions {
	return &GetKeyOptions{
		ID:              core.StringPtr(id),
		BluemixInstance: core.StringPtr(bluemixInstance),
	}
}

// SetID : Allow user to set ID
func (_options *GetKeyOptions) SetID(id string) *GetKeyOptions {
	_options.ID = core.StringPtr(id)
	return _options
}

// SetBluemixInstance : Allow user to set BluemixInstance
func (_options *GetKeyOptions) SetBluemixInstance(bluemixInstance string) *GetKeyOptions {
	_options.BluemixInstance = core.StringPtr(bluemixInstance)
	return _options
}

// SetCorrelationID : Allow user to set CorrelationID
func (_options *GetKeyOptions) SetCorrelationID(correlationID string) *GetKeyOptions {
	_options.CorrelationID = core.StringPtr(correlationID)
	return _options
}

// SetXKmsKeyRing : Allow user to set XKmsKeyRing
func (_options *GetKeyOptions) SetXKmsKeyRing(xKmsKeyRing string) *GetKeyOptions {
	_options.XKmsKeyRing = core.StringPtr(xKmsKeyRing)
	return _options
}

// SetHeaders : Allow user to set Headers
func (options *GetKeyOptions) SetHeaders(param map[string]string) *GetKeyOptions {
	options.Headers = param
	return options
}

// GetKeyPoliciesOneOf : GetKeyPoliciesOneOf struct
// Models which "extend" this model:
// - GetKeyPoliciesOneOfGetKeyPolicyDualAuthDelete
// - GetKeyPoliciesOneOfGetKeyPolicyRotation
// - GetKeyPoliciesOneOfGetMultipleKeyPolicies
type GetKeyPoliciesOneOf struct {
	// The metadata that describes the resource array.
	Metadata *CollectionMetadata `json:"metadata,omitempty"`

	// A collection of resources.
	Resources []GetKeyPoliciesOneOfResourcesItem `json:"resources,omitempty"`
}

func (*GetKeyPoliciesOneOf) isaGetKeyPoliciesOneOf() bool {
	return true
}

type GetKeyPoliciesOneOfIntf interface {
	isaGetKeyPoliciesOneOf() bool
}

// UnmarshalGetKeyPoliciesOneOf unmarshals an instance of GetKeyPoliciesOneOf from the specified map of raw messages.
func UnmarshalGetKeyPoliciesOneOf(m map[string]json.RawMessage, result interface{}) (err error) {
	obj := new(GetKeyPoliciesOneOf)
	err = core.UnmarshalModel(m, "metadata", &obj.Metadata, UnmarshalCollectionMetadata)
	if err != nil {
		err = core.SDKErrorf(err, "", "metadata-error", common.GetComponentInfo())
		return
	}
	err = core.UnmarshalModel(m, "resources", &obj.Resources, UnmarshalGetKeyPoliciesOneOfResourcesItem)
	if err != nil {
		err = core.SDKErrorf(err, "", "resources-error", common.GetComponentInfo())
		return
	}
	reflect.ValueOf(result).Elem().Set(reflect.ValueOf(obj))
	return
}

// GetKeyPoliciesOneOfGetKeyPolicyDualAuthDeleteResourcesItem : Properties that are associated with key level dual authorization delete policy.
type GetKeyPoliciesOneOfGetKeyPolicyDualAuthDeleteResourcesItem struct {
	// The v4 UUID used to uniquely identify the policy resource, as specified by RFC 4122.
	ID *string `json:"id,omitempty"`

	// The Cloud Resource Name (CRN) that uniquely identifies your cloud resources.
	Crn *string `json:"crn,omitempty"`

	// The date the policy was created. The date format follows RFC 3339.
	CreationDate *strfmt.DateTime `json:"creationDate,omitempty"`

	// The unique identifier for the resource that created the policy.
	CreatedBy *string `json:"createdBy,omitempty"`

	// Updates when the policy is replaced or modified. The date format follows RFC 3339.
	LastUpdateDate *strfmt.DateTime `json:"lastUpdateDate,omitempty"`

	// The unique identifier for the resource that updated the policy.
	UpdatedBy *string `json:"updatedBy,omitempty"`

	// Specifies the MIME type that represents the policy resource. Currently, only the default is supported.
	Type *string `json:"type" validate:"required"`

	// Data associated with the dual authorization delete policy.
	DualAuthDelete *KeyPolicyDualAuthDeleteDualAuthDelete `json:"dualAuthDelete" validate:"required"`
}

// Constants associated with the GetKeyPoliciesOneOfGetKeyPolicyDualAuthDeleteResourcesItem.Type property.
// Specifies the MIME type that represents the policy resource. Currently, only the default is supported.
const (
	GetKeyPoliciesOneOfGetKeyPolicyDualAuthDeleteResourcesItem_Type_ApplicationVndIbmKmsPolicyJSON = "application/vnd.ibm.kms.policy+json"
)

// UnmarshalGetKeyPoliciesOneOfGetKeyPolicyDualAuthDeleteResourcesItem unmarshals an instance of GetKeyPoliciesOneOfGetKeyPolicyDualAuthDeleteResourcesItem from the specified map of raw messages.
func UnmarshalGetKeyPoliciesOneOfGetKeyPolicyDualAuthDeleteResourcesItem(m map[string]json.RawMessage, result interface{}) (err error) {
	obj := new(GetKeyPoliciesOneOfGetKeyPolicyDualAuthDeleteResourcesItem)
	err = core.UnmarshalPrimitive(m, "id", &obj.ID)
	if err != nil {
		err = core.SDKErrorf(err, "", "id-error", common.GetComponentInfo())
		return
	}
	err = core.UnmarshalPrimitive(m, "crn", &obj.Crn)
	if err != nil {
		err = core.SDKErrorf(err, "", "crn-error", common.GetComponentInfo())
		return
	}
	err = core.UnmarshalPrimitive(m, "creationDate", &obj.CreationDate)
	if err != nil {
		err = core.SDKErrorf(err, "", "creationDate-error", common.GetComponentInfo())
		return
	}
	err = core.UnmarshalPrimitive(m, "createdBy", &obj.CreatedBy)
	if err != nil {
		err = core.SDKErrorf(err, "", "createdBy-error", common.GetComponentInfo())
		return
	}
	err = core.UnmarshalPrimitive(m, "lastUpdateDate", &obj.LastUpdateDate)
	if err != nil {
		err = core.SDKErrorf(err, "", "lastUpdateDate-error", common.GetComponentInfo())
		return
	}
	err = core.UnmarshalPrimitive(m, "updatedBy", &obj.UpdatedBy)
	if err != nil {
		err = core.SDKErrorf(err, "", "updatedBy-error", common.GetComponentInfo())
		return
	}
	err = core.UnmarshalPrimitive(m, "type", &obj.Type)
	if err != nil {
		err = core.SDKErrorf(err, "", "type-error", common.GetComponentInfo())
		return
	}
	err = core.UnmarshalModel(m, "dualAuthDelete", &obj.DualAuthDelete, UnmarshalKeyPolicyDualAuthDeleteDualAuthDelete)
	if err != nil {
		err = core.SDKErrorf(err, "", "dualAuthDelete-error", common.GetComponentInfo())
		return
	}
	reflect.ValueOf(result).Elem().Set(reflect.ValueOf(obj))
	return
}

// GetKeyPoliciesOneOfResourcesItem : Properties that are associated with key level dual authorization delete policy.
type GetKeyPoliciesOneOfResourcesItem struct {
	// The v4 UUID used to uniquely identify the policy resource, as specified by RFC 4122.
	ID *string `json:"id,omitempty"`

	// The Cloud Resource Name (CRN) that uniquely identifies your cloud resources.
	Crn *string `json:"crn,omitempty"`

	// The date the policy was created. The date format follows RFC 3339.
	CreationDate *strfmt.DateTime `json:"creationDate,omitempty"`

	// The unique identifier for the resource that created the policy.
	CreatedBy *string `json:"createdBy,omitempty"`

	// Updates when the policy is replaced or modified. The date format follows RFC 3339.
	LastUpdateDate *strfmt.DateTime `json:"lastUpdateDate,omitempty"`

	// The unique identifier for the resource that updated the policy.
	UpdatedBy *string `json:"updatedBy,omitempty"`

	// Specifies the MIME type that represents the policy resource. Currently, only the default is supported.
	Type *string `json:"type" validate:"required"`

	// Data associated with the dual authorization delete policy.
	DualAuthDelete *KeyPolicyDualAuthDeleteDualAuthDelete `json:"dualAuthDelete" validate:"required"`
}

// Constants associated with the GetKeyPoliciesOneOfResourcesItem.Type property.
// Specifies the MIME type that represents the policy resource. Currently, only the default is supported.
const (
	GetKeyPoliciesOneOfResourcesItem_Type_ApplicationVndIbmKmsPolicyJSON = "application/vnd.ibm.kms.policy+json"
)

// UnmarshalGetKeyPoliciesOneOfResourcesItem unmarshals an instance of GetKeyPoliciesOneOfResourcesItem from the specified map of raw messages.
func UnmarshalGetKeyPoliciesOneOfResourcesItem(m map[string]json.RawMessage, result interface{}) (err error) {
	obj := new(GetKeyPoliciesOneOfResourcesItem)
	err = core.UnmarshalPrimitive(m, "id", &obj.ID)
	if err != nil {
		err = core.SDKErrorf(err, "", "id-error", common.GetComponentInfo())
		return
	}
	err = core.UnmarshalPrimitive(m, "crn", &obj.Crn)
	if err != nil {
		err = core.SDKErrorf(err, "", "crn-error", common.GetComponentInfo())
		return
	}
	err = core.UnmarshalPrimitive(m, "creationDate", &obj.CreationDate)
	if err != nil {
		err = core.SDKErrorf(err, "", "creationDate-error", common.GetComponentInfo())
		return
	}
	err = core.UnmarshalPrimitive(m, "createdBy", &obj.CreatedBy)
	if err != nil {
		err = core.SDKErrorf(err, "", "createdBy-error", common.GetComponentInfo())
		return
	}
	err = core.UnmarshalPrimitive(m, "lastUpdateDate", &obj.LastUpdateDate)
	if err != nil {
		err = core.SDKErrorf(err, "", "lastUpdateDate-error", common.GetComponentInfo())
		return
	}
	err = core.UnmarshalPrimitive(m, "updatedBy", &obj.UpdatedBy)
	if err != nil {
		err = core.SDKErrorf(err, "", "updatedBy-error", common.GetComponentInfo())
		return
	}
	err = core.UnmarshalPrimitive(m, "type", &obj.Type)
	if err != nil {
		err = core.SDKErrorf(err, "", "type-error", common.GetComponentInfo())
		return
	}
	err = core.UnmarshalModel(m, "dualAuthDelete", &obj.DualAuthDelete, UnmarshalKeyPolicyDualAuthDeleteDualAuthDelete)
	if err != nil {
		err = core.SDKErrorf(err, "", "dualAuthDelete-error", common.GetComponentInfo())
		return
	}
	reflect.ValueOf(result).Elem().Set(reflect.ValueOf(obj))
	return
}

// GetKeyPolicyRotationResourcesItem : Properties that are associated with rotation policy.
type GetKeyPolicyRotationResourcesItem struct {
	// The v4 UUID used to uniquely identify the policy resource, as specified by RFC 4122.
	ID *string `json:"id,omitempty"`

	// The Cloud Resource Name (CRN) that uniquely identifies your cloud resources.
	Crn *string `json:"crn,omitempty"`

	// The date the policy was created. The date format follows RFC 3339.
	CreationDate *strfmt.DateTime `json:"creationDate,omitempty"`

	// The unique identifier for the resource that created the policy.
	CreatedBy *string `json:"createdBy,omitempty"`

	// Updates when the policy is replaced or modified. The date format follows RFC 3339.
	LastUpdateDate *strfmt.DateTime `json:"lastUpdateDate,omitempty"`

	// The unique identifier for the resource that updated the policy.
	UpdatedBy *string `json:"updatedBy,omitempty"`

	// Specifies the MIME type that represents the policy resource. Currently, only the default is supported.
	Type *string `json:"type" validate:"required"`

	// Data associated with the automatic key rotation policy.
	Rotation *KeyPolicyRotationRotation `json:"rotation" validate:"required"`
}

// Constants associated with the GetKeyPolicyRotationResourcesItem.Type property.
// Specifies the MIME type that represents the policy resource. Currently, only the default is supported.
const (
	GetKeyPolicyRotationResourcesItem_Type_ApplicationVndIbmKmsPolicyJSON = "application/vnd.ibm.kms.policy+json"
)

// UnmarshalGetKeyPolicyRotationResourcesItem unmarshals an instance of GetKeyPolicyRotationResourcesItem from the specified map of raw messages.
func UnmarshalGetKeyPolicyRotationResourcesItem(m map[string]json.RawMessage, result interface{}) (err error) {
	obj := new(GetKeyPolicyRotationResourcesItem)
	err = core.UnmarshalPrimitive(m, "id", &obj.ID)
	if err != nil {
		err = core.SDKErrorf(err, "", "id-error", common.GetComponentInfo())
		return
	}
	err = core.UnmarshalPrimitive(m, "crn", &obj.Crn)
	if err != nil {
		err = core.SDKErrorf(err, "", "crn-error", common.GetComponentInfo())
		return
	}
	err = core.UnmarshalPrimitive(m, "creationDate", &obj.CreationDate)
	if err != nil {
		err = core.SDKErrorf(err, "", "creationDate-error", common.GetComponentInfo())
		return
	}
	err = core.UnmarshalPrimitive(m, "createdBy", &obj.CreatedBy)
	if err != nil {
		err = core.SDKErrorf(err, "", "createdBy-error", common.GetComponentInfo())
		return
	}
	err = core.UnmarshalPrimitive(m, "lastUpdateDate", &obj.LastUpdateDate)
	if err != nil {
		err = core.SDKErrorf(err, "", "lastUpdateDate-error", common.GetComponentInfo())
		return
	}
	err = core.UnmarshalPrimitive(m, "updatedBy", &obj.UpdatedBy)
	if err != nil {
		err = core.SDKErrorf(err, "", "updatedBy-error", common.GetComponentInfo())
		return
	}
	err = core.UnmarshalPrimitive(m, "type", &obj.Type)
	if err != nil {
		err = core.SDKErrorf(err, "", "type-error", common.GetComponentInfo())
		return
	}
	err = core.UnmarshalModel(m, "rotation", &obj.Rotation, UnmarshalKeyPolicyRotationRotation)
	if err != nil {
		err = core.SDKErrorf(err, "", "rotation-error", common.GetComponentInfo())
		return
	}
	reflect.ValueOf(result).Elem().Set(reflect.ValueOf(obj))
	return
}

// GetKeyVersionsOptions : The GetKeyVersions options.
type GetKeyVersionsOptions struct {
	// The v4 UUID or alias that uniquely identifies the key.
	ID *string `json:"id" validate:"required,ne="`

	// The IBM Cloud instance ID that identifies your Key Protect service instance.
	BluemixInstance *string `json:"Bluemix-Instance" validate:"required"`

	// The v4 UUID used to correlate and track transactions.
	CorrelationID *string `json:"Correlation-Id,omitempty"`

	// The ID of the key ring that the specified key is a part of. When the header is not specified, Key Protect will
	// perform a key ring lookup. For a more optimized request, specify the key ring on every call. The key ring ID of keys
	// that are created without an `X-Kms-Key-Ring` header is: `default`.
	XKmsKeyRing *string `json:"X-Kms-Key-Ring,omitempty"`

	// The number of key versions to retrieve. By default, `GET /versions` returns the first 200 key versions. To retrieve
	// a different set of key versions, use `limit` with `offset` to page through your available resources. The maximum
	// value for `limit` is 5,000.
	// **Usage:** If you have a key with 20 versions in your instance, and you want to retrieve only the first 5 versions,
	// use `../versions?limit=5`.
	Limit *int64 `json:"limit,omitempty"`

	// The number of key versions to skip. By specifying `offset`, you retrieve a subset of key versions that starts with
	// the `offset` value. Use `offset` with `limit` to page through your available resources.
	// **Usage:** If you have a key with 100 versions in your instance, and you want to retrieve versions 26 through 50,
	// use `../versions?offset=25&limit=25`.
	Offset *int64 `json:"offset,omitempty"`

	// If set to `true`, returns `totalCount` in the response metadata for use with pagination. The `totalCount` value
	// returned specifies the total number of key versions that match the request, disregarding limit and offset. The
	// default is set to false.
	// **Usage:** To return the `totalCount` value for use with pagination, use `../versions?totalCount=true`.
	TotalCount *bool `json:"totalCount,omitempty"`

	// If set to `true`, returns the key versions of a key in any state. **Usage:** If you have deleted a key and still
	// want to retrieve its key versions use `../versions?allKeyStates=true`.
	AllKeyStates *bool `json:"allKeyStates,omitempty"`

	// Allows users to set headers on API requests.
	Headers map[string]string
}

// NewGetKeyVersionsOptions : Instantiate GetKeyVersionsOptions
func (*IbmKeyProtectApiV2) NewGetKeyVersionsOptions(id string, bluemixInstance string) *GetKeyVersionsOptions {
	return &GetKeyVersionsOptions{
		ID:              core.StringPtr(id),
		BluemixInstance: core.StringPtr(bluemixInstance),
	}
}

// SetID : Allow user to set ID
func (_options *GetKeyVersionsOptions) SetID(id string) *GetKeyVersionsOptions {
	_options.ID = core.StringPtr(id)
	return _options
}

// SetBluemixInstance : Allow user to set BluemixInstance
func (_options *GetKeyVersionsOptions) SetBluemixInstance(bluemixInstance string) *GetKeyVersionsOptions {
	_options.BluemixInstance = core.StringPtr(bluemixInstance)
	return _options
}

// SetCorrelationID : Allow user to set CorrelationID
func (_options *GetKeyVersionsOptions) SetCorrelationID(correlationID string) *GetKeyVersionsOptions {
	_options.CorrelationID = core.StringPtr(correlationID)
	return _options
}

// SetXKmsKeyRing : Allow user to set XKmsKeyRing
func (_options *GetKeyVersionsOptions) SetXKmsKeyRing(xKmsKeyRing string) *GetKeyVersionsOptions {
	_options.XKmsKeyRing = core.StringPtr(xKmsKeyRing)
	return _options
}

// SetLimit : Allow user to set Limit
func (_options *GetKeyVersionsOptions) SetLimit(limit int64) *GetKeyVersionsOptions {
	_options.Limit = core.Int64Ptr(limit)
	return _options
}

// SetOffset : Allow user to set Offset
func (_options *GetKeyVersionsOptions) SetOffset(offset int64) *GetKeyVersionsOptions {
	_options.Offset = core.Int64Ptr(offset)
	return _options
}

// SetTotalCount : Allow user to set TotalCount
func (_options *GetKeyVersionsOptions) SetTotalCount(totalCount bool) *GetKeyVersionsOptions {
	_options.TotalCount = core.BoolPtr(totalCount)
	return _options
}

// SetAllKeyStates : Allow user to set AllKeyStates
func (_options *GetKeyVersionsOptions) SetAllKeyStates(allKeyStates bool) *GetKeyVersionsOptions {
	_options.AllKeyStates = core.BoolPtr(allKeyStates)
	return _options
}

// SetHeaders : Allow user to set Headers
func (options *GetKeyVersionsOptions) SetHeaders(param map[string]string) *GetKeyVersionsOptions {
	options.Headers = param
	return options
}

// GetKeysOptions : The GetKeys options.
type GetKeysOptions struct {
	// The IBM Cloud instance ID that identifies your Key Protect service instance.
	BluemixInstance *string `json:"Bluemix-Instance" validate:"required"`

	// The v4 UUID used to correlate and track transactions.
	CorrelationID *string `json:"Correlation-Id,omitempty"`

	// The number of keys to retrieve. By default, `GET /keys` returns the first 200 keys. To retrieve a different set of
	// keys, use `limit` with `offset` to page through your available resources. The maximum value for `limit` is 5,000.
	// **Usage:** If you have 20 keys in your instance, and you want to retrieve only the first 5 keys, use
	// `../keys?limit=5`.
	Limit *int64 `json:"limit,omitempty"`

	// The number of keys to skip. By specifying `offset`, you retrieve a subset of keys that starts with the `offset`
	// value. Use `offset` with `limit` to page through your available resources.
	// **Usage:** If you have 100 keys in your instance, and you want to retrieve keys 26 through 50, use
	// `../keys?offset=25&limit=25`.
	Offset *int64 `json:"offset,omitempty"`

	// The state of the keys to be retrieved. States must be a list of integers from 0 to 5 delimited by commas with no
	// whitespace or trailing commas. Valid states are based on NIST SP 800-57. States are integers and correspond to the
	// Pre-activation = 0, Active = 1, Suspended = 2, Deactivated = 3, and Destroyed = 5 values.
	// **Usage:** If you want to retrieve active and deleted keys, use `../keys?state=1,5`.
	State []int64 `json:"state,omitempty"`

	// The type of keys to be retrieved. Filters keys based on the `extractable` property. You can use this query parameter
	// to search for keys whose material can leave the service. If set to `true`, standard keys will be retrieved. If set
	// to `false`, root keys will be retrieved. If omitted, both root and standard keys will be retrieved.
	// **Usage:** If you want to retrieve standard keys, use `../keys?extractable=true`.
	Extractable *bool `json:"extractable,omitempty"`

	// When provided, performs a search, possibly limiting the number of keys returned.
	// *Examples*:
	//
	//   - `foobar` - find keys where the name or any of its aliases contain `foobar`, case insentive (i.e. matches
	// `xfoobar`, `Foobar`).
	//   - `fadedbee-0000-0000-0000-1234567890ab` (a valid key id) - find keys where the id the key is
	// `fadedbee-0000-0000-0000-1234567890ab`, or the name or any of its aliases contain
	// `fadedbee-0000-0000-0000-1234567890ab`, case insentive.
	//
	// May prepend with options:
	//
	//   - `not:` = when specified, inverts matching logic (example: `not:foo` will search for keys that have aliases or
	// names that do not contain `foo`)
	//   - `escape:` = everything after this option is take as plaintext (example: `escape:not:` will search for keys that
	// have an alias or name containing the substring `not:`)
	//   - `exact:` = only looks for exact matches
	//
	// May prepend with search scopes:
	//
	//   - `alias:` = search in key aliases for search query
	//   - `name:` = search in key names for search query
	//
	// *Examples*:
	//
	//   - `not:exact:foobar`/`exact:not:foobar` - find keys where the name nor any of its aliases are *not* exactly
	// `foobar` (i.e. matches `xfoobar`, `bar`, `foo`)
	//   - `exact:escape:not:foobar` - find keys where the name or any of its aliases are exactly `not:foobar`
	//   - `not:alias:foobar`/`alias:not:foobar` - find keys where any of its aliases do *not* contain `foobar`
	//   - `name:exact:foobar`/`exact:name:foobar` - find keys where the name is exactly `foobar`
	//
	// *Note*:
	//
	//   By default, if no scopes are provided, search will be performed in both `name` and `alias` scopes.
	//
	//   Search is only possible on a intial searchable space of at most 5000 keys. If the initial seachable space is
	// greater than 5000 keys, the API returns HTTP 400 with the property resouces[0].reasons[0].code equals to
	// 'KEY_SEARCH_TOO_BROAD'.
	//   Use the following filters to reduce the initial searchable space:
	//
	//   - `state` (query parameter)
	//   - `extractable` (query parameter)
	//   - `X-Kms-Key-Ring` (HTTP header)
	//
	//   If the total intial searchable space exceeds the 5000 keys limit and when providing a fully specified key id or
	// when searching within the `alias` scope, a lookup
	//   will  be performed and if a key is found, the key will be returned as the only resource and in the response
	// metadata the property `incompleteSearch` will
	//   be `true`.
	//
	//   When providing a fully specified key id or when searching within the `alias` scope, a key lookup is performed in
	// addition to the search.
	//   This means search will try to lookup a single key that is uniquely identified by the key id or provided alias,
	// this key will be included in the response
	//   as the first resource, before other matches.
	//
	//   Search scopes are disjunctive, behaving in an *OR* manner. When using more than one search scope,
	//   a match in at least one of the scopes will result in the key being returned.
	Search *string `json:"search,omitempty"`

	// When provided, sorts the list of keys returned based on one or more key properties. To sort on a property in
	// descending order, prefix the term with "-". To sort on multiple key properties, use a comma to separate each
	// properties. The first property in the comma-separated list will be evaluated before the next. The key properties
	// that can be sorted at this time are:
	//   - `id`
	//   - `state`
	//   - `extractable`
	//   - `imported`
	//   - `creationDate`
	//   - `lastUpdateDate`
	//   - `lastRotateDate`
	//   - `deletionDate`
	//   - `expirationDate`
	//
	// The list of keys returned is sorted on id by default, if this parameter is not provided.
	Sort *string `json:"sort,omitempty"`

	// When provided, returns the list of keys that match the queried properties. Each key property to be filtered on is
	// specified as the property name itself, followed by an “=“ symbol, and then the value to filter on, followed by a
	// space if there are more properties to filter only. Note: Anything between `<` and `>` in the examples or
	// descriptions represent placeholder to specify the value
	// *Basic format*: <propertyA>=<valueB> <propertyB>=<valueB> - The value to filter on may contain a value related to
	// the property itself, or an operator followed by a value accepted by the operator - Only one operator and value, or
	// one value is accepted per property at a time
	// *Format with operator/value pair*: <propertyA>=<operatorA>:<valueA> Up to three of the same property may be
	// specified at a time. The key properties that can be filtered at this time are:
	// - `creationDate`
	//   * Date in RFC 3339 format in double-quotes: “2000-03-21T00:00:00Z”
	// - `deletionDate`
	//   * Date in RFC 3339 format in double-quotes: “2000-03-21T00:00:00Z”
	// - `expirationDate`
	//   * Date in RFC 3339 format in double-quotes: “2000-03-21T00:00:00Z”
	// - `extractable`
	//   * Boolean true or false without quotes, case-insensitive
	// - `lastRotateDate`
	//   * Date in RFC 3339 format in double-quotes: “2000-03-21T00:00:00Z”
	// - `lastUpdateDate`
	//   * Date in RFC 3339 format in double-quotes: “2000-03-21T00:00:00Z”
	// - `state`
	//   * A list of comma-separated integers with no space in between: 0,1,2,3,5 Comparison operations (operators) that
	// can be performed on date values are:
	// - `lte:<value>` Less than or equal to - `lt:<value>` Less than - `gte:<value>` Greater than or equal to -
	// `gt:<value>` Greater than A special keyword for date, `none` (case-insensitive), may be used to retreive keys that
	// do not have that property. This is useful for `lastRotateDate`, where only keys that have never been rotated can be
	// retreived.
	// *Examples*:
	// - `lastRotateDate="2022-02-15T00:00:00Z"` Filter keys that were last rotated on February 15, 2022 -
	// `lastRotateDate=gte:"2022-02-15T00:00:00Z"` Filter keys that were last rotated after or on February 15, 2022 -
	// `lastRotateDate=gte:"2022-02-15T00:00:00Z" lastRotateDate=lt:"2022-03-15T00:00:00Z"` Filter keys that were last
	// rotated after or on February 15, 2022 but before (not including) March 15, 2022 -
	// `lastRotateDate="2022-02-15T00:00:00Z" state=0,1,2,3,5 extractable=false` Filter root keys that were last rotated on
	// February 15, 2022, with any state
	// *Note*: When you filter by `state` or `extractable` in this query parameter, you will not be able to use the
	// deprecated `state` or `extractable` independent query parameter. You will get a 400 response code if you specify a
	// value for one of the two properties in both this filter query parameter and the deprecated independent query of the
	// same name (the same applies vice versa).
	Filter *string `json:"filter,omitempty"`

	// The ID of the target key ring. If unspecified, all resources in the instance that the caller has access to will be
	// returned. When the header is specified, only resources within the specified key ring, that the caller has access to,
	// will be returned. The key ring ID of keys that are created without an `X-Kms-Key-Ring` header is: `default`.
	XKmsKeyRing *string `json:"X-Kms-Key-Ring,omitempty"`

	// Allows users to set headers on API requests.
	Headers map[string]string
}

// Constants associated with the GetKeysOptions.Sort property.
// When provided, sorts the list of keys returned based on one or more key properties. To sort on a property in
// descending order, prefix the term with "-". To sort on multiple key properties, use a comma to separate each
// properties. The first property in the comma-separated list will be evaluated before the next. The key properties that
// can be sorted at this time are:
//   - `id`
//   - `state`
//   - `extractable`
//   - `imported`
//   - `creationDate`
//   - `lastUpdateDate`
//   - `lastRotateDate`
//   - `deletionDate`
//   - `expirationDate`
//
// The list of keys returned is sorted on id by default, if this parameter is not provided.
const (
	GetKeysOptions_Sort_Creationdate   = "creationDate"
	GetKeysOptions_Sort_Deletiondate   = "deletionDate"
	GetKeysOptions_Sort_Expirationdate = "expirationDate"
	GetKeysOptions_Sort_Extractable    = "extractable"
	GetKeysOptions_Sort_ID             = "id"
	GetKeysOptions_Sort_Imported       = "imported"
	GetKeysOptions_Sort_Lastrotatedate = "lastRotateDate"
	GetKeysOptions_Sort_Lastupdatedate = "lastUpdateDate"
	GetKeysOptions_Sort_State          = "state"
)

// NewGetKeysOptions : Instantiate GetKeysOptions
func (*IbmKeyProtectApiV2) NewGetKeysOptions(bluemixInstance string) *GetKeysOptions {
	return &GetKeysOptions{
		BluemixInstance: core.StringPtr(bluemixInstance),
	}
}

// SetBluemixInstance : Allow user to set BluemixInstance
func (_options *GetKeysOptions) SetBluemixInstance(bluemixInstance string) *GetKeysOptions {
	_options.BluemixInstance = core.StringPtr(bluemixInstance)
	return _options
}

// SetCorrelationID : Allow user to set CorrelationID
func (_options *GetKeysOptions) SetCorrelationID(correlationID string) *GetKeysOptions {
	_options.CorrelationID = core.StringPtr(correlationID)
	return _options
}

// SetLimit : Allow user to set Limit
func (_options *GetKeysOptions) SetLimit(limit int64) *GetKeysOptions {
	_options.Limit = core.Int64Ptr(limit)
	return _options
}

// SetOffset : Allow user to set Offset
func (_options *GetKeysOptions) SetOffset(offset int64) *GetKeysOptions {
	_options.Offset = core.Int64Ptr(offset)
	return _options
}

// SetState : Allow user to set State
func (_options *GetKeysOptions) SetState(state []int64) *GetKeysOptions {
	_options.State = state
	return _options
}

// SetExtractable : Allow user to set Extractable
func (_options *GetKeysOptions) SetExtractable(extractable bool) *GetKeysOptions {
	_options.Extractable = core.BoolPtr(extractable)
	return _options
}

// SetSearch : Allow user to set Search
func (_options *GetKeysOptions) SetSearch(search string) *GetKeysOptions {
	_options.Search = core.StringPtr(search)
	return _options
}

// SetSort : Allow user to set Sort
func (_options *GetKeysOptions) SetSort(sort string) *GetKeysOptions {
	_options.Sort = core.StringPtr(sort)
	return _options
}

// SetFilter : Allow user to set Filter
func (_options *GetKeysOptions) SetFilter(filter string) *GetKeysOptions {
	_options.Filter = core.StringPtr(filter)
	return _options
}

// SetXKmsKeyRing : Allow user to set XKmsKeyRing
func (_options *GetKeysOptions) SetXKmsKeyRing(xKmsKeyRing string) *GetKeysOptions {
	_options.XKmsKeyRing = core.StringPtr(xKmsKeyRing)
	return _options
}

// SetHeaders : Allow user to set Headers
func (options *GetKeysOptions) SetHeaders(param map[string]string) *GetKeysOptions {
	options.Headers = param
	return options
}

// GetKmipAdapterOptions : The GetKmipAdapter options.
type GetKmipAdapterOptions struct {
	// The name or v4 UUID of the KMIP Adapter that uniquely identifies it.
	ID *string `json:"id" validate:"required,ne="`

	// The IBM Cloud instance ID that identifies your Key Protect service instance.
	BluemixInstance *string `json:"Bluemix-Instance" validate:"required"`

	// The v4 UUID used to correlate and track transactions.
	CorrelationID *string `json:"Correlation-Id,omitempty"`

	// Allows users to set headers on API requests.
	Headers map[string]string
}

// NewGetKmipAdapterOptions : Instantiate GetKmipAdapterOptions
func (*IbmKeyProtectApiV2) NewGetKmipAdapterOptions(id string, bluemixInstance string) *GetKmipAdapterOptions {
	return &GetKmipAdapterOptions{
		ID:              core.StringPtr(id),
		BluemixInstance: core.StringPtr(bluemixInstance),
	}
}

// SetID : Allow user to set ID
func (_options *GetKmipAdapterOptions) SetID(id string) *GetKmipAdapterOptions {
	_options.ID = core.StringPtr(id)
	return _options
}

// SetBluemixInstance : Allow user to set BluemixInstance
func (_options *GetKmipAdapterOptions) SetBluemixInstance(bluemixInstance string) *GetKmipAdapterOptions {
	_options.BluemixInstance = core.StringPtr(bluemixInstance)
	return _options
}

// SetCorrelationID : Allow user to set CorrelationID
func (_options *GetKmipAdapterOptions) SetCorrelationID(correlationID string) *GetKmipAdapterOptions {
	_options.CorrelationID = core.StringPtr(correlationID)
	return _options
}

// SetHeaders : Allow user to set Headers
func (options *GetKmipAdapterOptions) SetHeaders(param map[string]string) *GetKmipAdapterOptions {
	options.Headers = param
	return options
}

// GetKmipAdaptersOptions : The GetKmipAdapters options.
type GetKmipAdaptersOptions struct {
	// The IBM Cloud instance ID that identifies your Key Protect service instance.
	BluemixInstance *string `json:"Bluemix-Instance" validate:"required"`

	// The v4 UUID used to correlate and track transactions.
	CorrelationID *string `json:"Correlation-Id,omitempty"`

	// The number of KMIP Adapters to retrieve. By default, `GET /kmip_adapters` returns the first 100 KMIP Adapters. To
	// retrieve a different set of KMIP adapters, use `limit` with `offset` to page through your available resources. The
	// maximum value for `limit` is 200.
	// **Usage:** If you have 20 KMIP Adapters, and you want to retrieve only the first 5 adapters, use
	// `../kmip_adapters?limit=5`.
	Limit *int64 `json:"limit,omitempty"`

	// The number of KMIP adapters to skip. By specifying `offset`, you retrieve a subset of KMIP adapters that starts with
	// the `offset` value. Use `offset` with `limit` to page through your available resources.
	// **Usage:** If you have 20 KMIP Adapters, and you want to retrieve adapters 11 through 15, use
	// `../kmip_adapters?offset=10&limit=5`.
	Offset *int64 `json:"offset,omitempty"`

	// If set to `true`, returns `totalCount` in the response metadata for use with pagination. The `totalCount` value
	// returned specifies the total number of kmip adapters that match the request, disregarding limit and offset. The
	// default is set to false. **Usage:** To return the `totalCount` value for use with pagination, use
	// `../kmip_adapters?totalCount=true`.
	TotalCount *bool `json:"totalCount,omitempty"`

	// The root key ID(`crk_id`) in the `profile_data` to filter on. This field is currently only applicable to profile
	// `"native_1.0"`. It will only return adapters with profile_data that contains this field. Example usage
	// `../kmip_adapters?crk_id=feddecaf-0000-0000-0000-1234567890ab`.
	CrkID *string `json:"crk_id,omitempty"`

	// Allows users to set headers on API requests.
	Headers map[string]string
}

// NewGetKmipAdaptersOptions : Instantiate GetKmipAdaptersOptions
func (*IbmKeyProtectApiV2) NewGetKmipAdaptersOptions(bluemixInstance string) *GetKmipAdaptersOptions {
	return &GetKmipAdaptersOptions{
		BluemixInstance: core.StringPtr(bluemixInstance),
	}
}

// SetBluemixInstance : Allow user to set BluemixInstance
func (_options *GetKmipAdaptersOptions) SetBluemixInstance(bluemixInstance string) *GetKmipAdaptersOptions {
	_options.BluemixInstance = core.StringPtr(bluemixInstance)
	return _options
}

// SetCorrelationID : Allow user to set CorrelationID
func (_options *GetKmipAdaptersOptions) SetCorrelationID(correlationID string) *GetKmipAdaptersOptions {
	_options.CorrelationID = core.StringPtr(correlationID)
	return _options
}

// SetLimit : Allow user to set Limit
func (_options *GetKmipAdaptersOptions) SetLimit(limit int64) *GetKmipAdaptersOptions {
	_options.Limit = core.Int64Ptr(limit)
	return _options
}

// SetOffset : Allow user to set Offset
func (_options *GetKmipAdaptersOptions) SetOffset(offset int64) *GetKmipAdaptersOptions {
	_options.Offset = core.Int64Ptr(offset)
	return _options
}

// SetTotalCount : Allow user to set TotalCount
func (_options *GetKmipAdaptersOptions) SetTotalCount(totalCount bool) *GetKmipAdaptersOptions {
	_options.TotalCount = core.BoolPtr(totalCount)
	return _options
}

// SetCrkID : Allow user to set CrkID
func (_options *GetKmipAdaptersOptions) SetCrkID(crkID string) *GetKmipAdaptersOptions {
	_options.CrkID = core.StringPtr(crkID)
	return _options
}

// SetHeaders : Allow user to set Headers
func (options *GetKmipAdaptersOptions) SetHeaders(param map[string]string) *GetKmipAdaptersOptions {
	options.Headers = param
	return options
}

// GetKmipClientCertificateOptions : The GetKmipClientCertificate options.
type GetKmipClientCertificateOptions struct {
	// The name or v4 UUID of the KMIP Adapter that uniquely identifies it.
	AdapterID *string `json:"adapter_id" validate:"required,ne="`

	// The name or v4 UUID of the client certificate that uniquely identifies it.
	ID *string `json:"id" validate:"required,ne="`

	// The IBM Cloud instance ID that identifies your Key Protect service instance.
	BluemixInstance *string `json:"Bluemix-Instance" validate:"required"`

	// The v4 UUID used to correlate and track transactions.
	CorrelationID *string `json:"Correlation-Id,omitempty"`

	// Allows users to set headers on API requests.
	Headers map[string]string
}

// NewGetKmipClientCertificateOptions : Instantiate GetKmipClientCertificateOptions
func (*IbmKeyProtectApiV2) NewGetKmipClientCertificateOptions(adapterID string, id string, bluemixInstance string) *GetKmipClientCertificateOptions {
	return &GetKmipClientCertificateOptions{
		AdapterID:       core.StringPtr(adapterID),
		ID:              core.StringPtr(id),
		BluemixInstance: core.StringPtr(bluemixInstance),
	}
}

// SetAdapterID : Allow user to set AdapterID
func (_options *GetKmipClientCertificateOptions) SetAdapterID(adapterID string) *GetKmipClientCertificateOptions {
	_options.AdapterID = core.StringPtr(adapterID)
	return _options
}

// SetID : Allow user to set ID
func (_options *GetKmipClientCertificateOptions) SetID(id string) *GetKmipClientCertificateOptions {
	_options.ID = core.StringPtr(id)
	return _options
}

// SetBluemixInstance : Allow user to set BluemixInstance
func (_options *GetKmipClientCertificateOptions) SetBluemixInstance(bluemixInstance string) *GetKmipClientCertificateOptions {
	_options.BluemixInstance = core.StringPtr(bluemixInstance)
	return _options
}

// SetCorrelationID : Allow user to set CorrelationID
func (_options *GetKmipClientCertificateOptions) SetCorrelationID(correlationID string) *GetKmipClientCertificateOptions {
	_options.CorrelationID = core.StringPtr(correlationID)
	return _options
}

// SetHeaders : Allow user to set Headers
func (options *GetKmipClientCertificateOptions) SetHeaders(param map[string]string) *GetKmipClientCertificateOptions {
	options.Headers = param
	return options
}

// GetKmipClientCertificatesOptions : The GetKmipClientCertificates options.
type GetKmipClientCertificatesOptions struct {
	// The name or v4 UUID of the KMIP Adapter that uniquely identifies it.
	AdapterID *string `json:"adapter_id" validate:"required,ne="`

	// The IBM Cloud instance ID that identifies your Key Protect service instance.
	BluemixInstance *string `json:"Bluemix-Instance" validate:"required"`

	// The number of client certificates to retrieve. By default, `GET /kmip_adapters/{id}/certificates` returns the first
	// 100 certificates. To retrieve a different set of certificates, use `limit` with `offset` to page through your
	// available resources. The maximum value for `limit` is 200.
	// **Usage:** If you have 20 certificates associated with your KMIP adapter, and you want to retrieve only the first 5
	// certificates, use `../kmip_adapters/{id}/certificates?limit=5`.
	Limit *int64 `json:"limit,omitempty"`

	// The number of client certificates to skip. By specifying `offset`, you retrieve a subset of certificates that starts
	// with the `offset` value. Use `offset` with `limit` to page through your available resources.
	// **Usage:** If you have 20 certificates associated with your KMIP adapter, and you want to retrieve certificates 11
	// through 15, use `../kmip_adapters/{id}/certificates?offset=10&limit=5`.
	Offset *int64 `json:"offset,omitempty"`

	// If set to `true`, returns `totalCount` in the response metadata for use with pagination. The `totalCount` value
	// returned specifies the total number of client certificates that match the request, disregarding limit and offset.
	// The default is set to false. **Usage:** To return the `totalCount` value for use with pagination, use
	// `../kmip_adapters/{id}/certificates?totalCount=true`.
	TotalCount *bool `json:"totalCount,omitempty"`

	// The v4 UUID used to correlate and track transactions.
	CorrelationID *string `json:"Correlation-Id,omitempty"`

	// Allows users to set headers on API requests.
	Headers map[string]string
}

// NewGetKmipClientCertificatesOptions : Instantiate GetKmipClientCertificatesOptions
func (*IbmKeyProtectApiV2) NewGetKmipClientCertificatesOptions(adapterID string, bluemixInstance string) *GetKmipClientCertificatesOptions {
	return &GetKmipClientCertificatesOptions{
		AdapterID:       core.StringPtr(adapterID),
		BluemixInstance: core.StringPtr(bluemixInstance),
	}
}

// SetAdapterID : Allow user to set AdapterID
func (_options *GetKmipClientCertificatesOptions) SetAdapterID(adapterID string) *GetKmipClientCertificatesOptions {
	_options.AdapterID = core.StringPtr(adapterID)
	return _options
}

// SetBluemixInstance : Allow user to set BluemixInstance
func (_options *GetKmipClientCertificatesOptions) SetBluemixInstance(bluemixInstance string) *GetKmipClientCertificatesOptions {
	_options.BluemixInstance = core.StringPtr(bluemixInstance)
	return _options
}

// SetLimit : Allow user to set Limit
func (_options *GetKmipClientCertificatesOptions) SetLimit(limit int64) *GetKmipClientCertificatesOptions {
	_options.Limit = core.Int64Ptr(limit)
	return _options
}

// SetOffset : Allow user to set Offset
func (_options *GetKmipClientCertificatesOptions) SetOffset(offset int64) *GetKmipClientCertificatesOptions {
	_options.Offset = core.Int64Ptr(offset)
	return _options
}

// SetTotalCount : Allow user to set TotalCount
func (_options *GetKmipClientCertificatesOptions) SetTotalCount(totalCount bool) *GetKmipClientCertificatesOptions {
	_options.TotalCount = core.BoolPtr(totalCount)
	return _options
}

// SetCorrelationID : Allow user to set CorrelationID
func (_options *GetKmipClientCertificatesOptions) SetCorrelationID(correlationID string) *GetKmipClientCertificatesOptions {
	_options.CorrelationID = core.StringPtr(correlationID)
	return _options
}

// SetHeaders : Allow user to set Headers
func (options *GetKmipClientCertificatesOptions) SetHeaders(param map[string]string) *GetKmipClientCertificatesOptions {
	options.Headers = param
	return options
}

// GetKmipObjectOptions : The GetKmipObject options.
type GetKmipObjectOptions struct {
	// The name or v4 UUID of the KMIP Adapter that uniquely identifies it.
	AdapterID *string `json:"adapter_id" validate:"required,ne="`

	// The IBM Cloud instance ID that identifies your Key Protect service instance.
	BluemixInstance *string `json:"Bluemix-Instance" validate:"required"`

	// The v4 UUID of the kmip object that uniquely identifies it.
	ID *string `json:"id" validate:"required,ne="`

	// The v4 UUID used to correlate and track transactions.
	CorrelationID *string `json:"Correlation-Id,omitempty"`

	// Allows users to set headers on API requests.
	Headers map[string]string
}

// NewGetKmipObjectOptions : Instantiate GetKmipObjectOptions
func (*IbmKeyProtectApiV2) NewGetKmipObjectOptions(adapterID string, bluemixInstance string, id string) *GetKmipObjectOptions {
	return &GetKmipObjectOptions{
		AdapterID:       core.StringPtr(adapterID),
		BluemixInstance: core.StringPtr(bluemixInstance),
		ID:              core.StringPtr(id),
	}
}

// SetAdapterID : Allow user to set AdapterID
func (_options *GetKmipObjectOptions) SetAdapterID(adapterID string) *GetKmipObjectOptions {
	_options.AdapterID = core.StringPtr(adapterID)
	return _options
}

// SetBluemixInstance : Allow user to set BluemixInstance
func (_options *GetKmipObjectOptions) SetBluemixInstance(bluemixInstance string) *GetKmipObjectOptions {
	_options.BluemixInstance = core.StringPtr(bluemixInstance)
	return _options
}

// SetID : Allow user to set ID
func (_options *GetKmipObjectOptions) SetID(id string) *GetKmipObjectOptions {
	_options.ID = core.StringPtr(id)
	return _options
}

// SetCorrelationID : Allow user to set CorrelationID
func (_options *GetKmipObjectOptions) SetCorrelationID(correlationID string) *GetKmipObjectOptions {
	_options.CorrelationID = core.StringPtr(correlationID)
	return _options
}

// SetHeaders : Allow user to set Headers
func (options *GetKmipObjectOptions) SetHeaders(param map[string]string) *GetKmipObjectOptions {
	options.Headers = param
	return options
}

// GetKmipObjectsOptions : The GetKmipObjects options.
type GetKmipObjectsOptions struct {
	// The name or v4 UUID of the KMIP Adapter that uniquely identifies it.
	AdapterID *string `json:"adapter_id" validate:"required,ne="`

	// The IBM Cloud instance ID that identifies your Key Protect service instance.
	BluemixInstance *string `json:"Bluemix-Instance" validate:"required"`

	// The number of kmip objects to retrieve. By default, `GET /kmip_adapters/{id}/kmip_objects` returns the first 100
	// kmip_objects. To retrieve a different set of kmip objects, use `limit` with `offset` to page through your available
	// resources. The maximum value for `limit` is 5000.
	// **Usage:** If you have 20 kmip objects associated with your KMIP adapter, and you want to retrieve only the first 5
	// kmip objects, use `../kmip_adapters/{id}/kmip_objects?limit=5`.
	Limit *int64 `json:"limit,omitempty"`

	// The number of kmip objects to skip. By specifying `offset`, you retrieve a subset of kmip objects that starts with
	// the `offset` value. Use `offset` with `limit` to page through your available resources.
	// **Usage:** If you have 20 kmip objects associated with your KMIP adapter, and you want to retrieve kmip objects 11
	// through 15, use `../kmip_adapters/{id}/kmip_objects?offset=10&limit=5`.
	Offset *int64 `json:"offset,omitempty"`

	// If set to `true`, returns `totalCount` in the response metadata for use with pagination. The `totalCount` value
	// returned specifies the total number of kmip objects that match the request, disregarding limit and offset. The
	// default is set to false. **Usage:** To return the `totalCount` value for use with pagination, use
	// `../kmip_adapters/{id}/kmip_objects?totalCount=true`.
	TotalCount *bool `json:"totalCount,omitempty"`

	// List of states to filter the KMIP objects on. The `default` is set to `[1,2,3,4]`. States are integers and
	// correspond to Pre-Active = 1, Active = 2, Deactivated = 3, Compromised = 4, Destroyed = 5, Destroyed Compromised =
	// 6. **Usage:** To filter on multiples `state` values, use `../kmip_adapters/{id}/kmip_objects?state=2,3`.
	State []int64 `json:"state,omitempty"`

	// The v4 UUID used to correlate and track transactions.
	CorrelationID *string `json:"Correlation-Id,omitempty"`

	// Allows users to set headers on API requests.
	Headers map[string]string
}

// NewGetKmipObjectsOptions : Instantiate GetKmipObjectsOptions
func (*IbmKeyProtectApiV2) NewGetKmipObjectsOptions(adapterID string, bluemixInstance string) *GetKmipObjectsOptions {
	return &GetKmipObjectsOptions{
		AdapterID:       core.StringPtr(adapterID),
		BluemixInstance: core.StringPtr(bluemixInstance),
	}
}

// SetAdapterID : Allow user to set AdapterID
func (_options *GetKmipObjectsOptions) SetAdapterID(adapterID string) *GetKmipObjectsOptions {
	_options.AdapterID = core.StringPtr(adapterID)
	return _options
}

// SetBluemixInstance : Allow user to set BluemixInstance
func (_options *GetKmipObjectsOptions) SetBluemixInstance(bluemixInstance string) *GetKmipObjectsOptions {
	_options.BluemixInstance = core.StringPtr(bluemixInstance)
	return _options
}

// SetLimit : Allow user to set Limit
func (_options *GetKmipObjectsOptions) SetLimit(limit int64) *GetKmipObjectsOptions {
	_options.Limit = core.Int64Ptr(limit)
	return _options
}

// SetOffset : Allow user to set Offset
func (_options *GetKmipObjectsOptions) SetOffset(offset int64) *GetKmipObjectsOptions {
	_options.Offset = core.Int64Ptr(offset)
	return _options
}

// SetTotalCount : Allow user to set TotalCount
func (_options *GetKmipObjectsOptions) SetTotalCount(totalCount bool) *GetKmipObjectsOptions {
	_options.TotalCount = core.BoolPtr(totalCount)
	return _options
}

// SetState : Allow user to set State
func (_options *GetKmipObjectsOptions) SetState(state []int64) *GetKmipObjectsOptions {
	_options.State = state
	return _options
}

// SetCorrelationID : Allow user to set CorrelationID
func (_options *GetKmipObjectsOptions) SetCorrelationID(correlationID string) *GetKmipObjectsOptions {
	_options.CorrelationID = core.StringPtr(correlationID)
	return _options
}

// SetHeaders : Allow user to set Headers
func (options *GetKmipObjectsOptions) SetHeaders(param map[string]string) *GetKmipObjectsOptions {
	options.Headers = param
	return options
}

// GetMultipleKeyPoliciesResource : Properties that are associated with rotation policy.
type GetMultipleKeyPoliciesResource struct {
	// Data associated with the dual authorization delete policy.
	DualAuthDelete *GetMultipleKeyPoliciesResourceDualAuthDelete `json:"dualAuthDelete,omitempty"`

	// Data associated with the automatic key rotation policy.
	Rotation *KeyPolicyRotationNonRequiredRotation `json:"rotation,omitempty"`

	// The v4 UUID used to uniquely identify the policy resource, as specified by RFC 4122.
	ID *string `json:"id,omitempty"`

	// The Cloud Resource Name (CRN) that uniquely identifies your cloud resources.
	Crn *string `json:"crn,omitempty"`

	// The date the policy was created. The date format follows RFC 3339.
	CreationDate *strfmt.DateTime `json:"creationDate,omitempty"`

	// The unique identifier for the resource that created the policy.
	CreatedBy *string `json:"createdBy,omitempty"`

	// Updates when the policy is replaced or modified. The date format follows RFC 3339.
	LastUpdateDate *strfmt.DateTime `json:"lastUpdateDate,omitempty"`

	// The unique identifier for the resource that updated the policy.
	UpdatedBy *string `json:"updatedBy,omitempty"`
}

// UnmarshalGetMultipleKeyPoliciesResource unmarshals an instance of GetMultipleKeyPoliciesResource from the specified map of raw messages.
func UnmarshalGetMultipleKeyPoliciesResource(m map[string]json.RawMessage, result interface{}) (err error) {
	obj := new(GetMultipleKeyPoliciesResource)
	err = core.UnmarshalModel(m, "dualAuthDelete", &obj.DualAuthDelete, UnmarshalGetMultipleKeyPoliciesResourceDualAuthDelete)
	if err != nil {
		err = core.SDKErrorf(err, "", "dualAuthDelete-error", common.GetComponentInfo())
		return
	}
	err = core.UnmarshalModel(m, "rotation", &obj.Rotation, UnmarshalKeyPolicyRotationNonRequiredRotation)
	if err != nil {
		err = core.SDKErrorf(err, "", "rotation-error", common.GetComponentInfo())
		return
	}
	err = core.UnmarshalPrimitive(m, "id", &obj.ID)
	if err != nil {
		err = core.SDKErrorf(err, "", "id-error", common.GetComponentInfo())
		return
	}
	err = core.UnmarshalPrimitive(m, "crn", &obj.Crn)
	if err != nil {
		err = core.SDKErrorf(err, "", "crn-error", common.GetComponentInfo())
		return
	}
	err = core.UnmarshalPrimitive(m, "creationDate", &obj.CreationDate)
	if err != nil {
		err = core.SDKErrorf(err, "", "creationDate-error", common.GetComponentInfo())
		return
	}
	err = core.UnmarshalPrimitive(m, "createdBy", &obj.CreatedBy)
	if err != nil {
		err = core.SDKErrorf(err, "", "createdBy-error", common.GetComponentInfo())
		return
	}
	err = core.UnmarshalPrimitive(m, "lastUpdateDate", &obj.LastUpdateDate)
	if err != nil {
		err = core.SDKErrorf(err, "", "lastUpdateDate-error", common.GetComponentInfo())
		return
	}
	err = core.UnmarshalPrimitive(m, "updatedBy", &obj.UpdatedBy)
	if err != nil {
		err = core.SDKErrorf(err, "", "updatedBy-error", common.GetComponentInfo())
		return
	}
	reflect.ValueOf(result).Elem().Set(reflect.ValueOf(obj))
	return
}

// GetMultipleKeyPoliciesResourceDualAuthDelete : Data associated with the dual authorization delete policy.
type GetMultipleKeyPoliciesResourceDualAuthDelete struct {
	// If set to `true`, Key Protect enables a dual authorization policy on a single key. After you enable the policy, Key
	// Protect requires an authorization from two users to delete this key. For example, you can authorize the deletion
	// first by using the [SetKeyForDeletion](#invoke-an-action-on-a-key) action. Then, a different user provides a second
	// authorization implicitly by calling `DELETE /keys` to delete the key.
	// **Note:** Once the dual authorization policy is set on the key, it cannot be reverted.
	Enabled *bool `json:"enabled" validate:"required"`
}

// UnmarshalGetMultipleKeyPoliciesResourceDualAuthDelete unmarshals an instance of GetMultipleKeyPoliciesResourceDualAuthDelete from the specified map of raw messages.
func UnmarshalGetMultipleKeyPoliciesResourceDualAuthDelete(m map[string]json.RawMessage, result interface{}) (err error) {
	obj := new(GetMultipleKeyPoliciesResourceDualAuthDelete)
	err = core.UnmarshalPrimitive(m, "enabled", &obj.Enabled)
	if err != nil {
		err = core.SDKErrorf(err, "", "enabled-error", common.GetComponentInfo())
		return
	}
	reflect.ValueOf(result).Elem().Set(reflect.ValueOf(obj))
	return
}

// GetPolicyOptions : The GetPolicy options.
type GetPolicyOptions struct {
	// The v4 UUID or alias that uniquely identifies the key.
	ID *string `json:"id" validate:"required,ne="`

	// The IBM Cloud instance ID that identifies your Key Protect service instance.
	BluemixInstance *string `json:"Bluemix-Instance" validate:"required"`

	// The v4 UUID used to correlate and track transactions.
	CorrelationID *string `json:"Correlation-Id,omitempty"`

	// The ID of the key ring that the specified key is a part of. When the header is not specified, Key Protect will
	// perform a key ring lookup. For a more optimized request, specify the key ring on every call. The key ring ID of keys
	// that are created without an `X-Kms-Key-Ring` header is: `default`.
	XKmsKeyRing *string `json:"X-Kms-Key-Ring,omitempty"`

	// The type of policy that is associated with the specified key.
	Policy *string `json:"policy,omitempty"`

	// Allows users to set headers on API requests.
	Headers map[string]string
}

// Constants associated with the GetPolicyOptions.Policy property.
// The type of policy that is associated with the specified key.
const (
	GetPolicyOptions_Policy_Dualauthdelete = "dualAuthDelete"
	GetPolicyOptions_Policy_Rotation       = "rotation"
)

// NewGetPolicyOptions : Instantiate GetPolicyOptions
func (*IbmKeyProtectApiV2) NewGetPolicyOptions(id string, bluemixInstance string) *GetPolicyOptions {
	return &GetPolicyOptions{
		ID:              core.StringPtr(id),
		BluemixInstance: core.StringPtr(bluemixInstance),
	}
}

// SetID : Allow user to set ID
func (_options *GetPolicyOptions) SetID(id string) *GetPolicyOptions {
	_options.ID = core.StringPtr(id)
	return _options
}

// SetBluemixInstance : Allow user to set BluemixInstance
func (_options *GetPolicyOptions) SetBluemixInstance(bluemixInstance string) *GetPolicyOptions {
	_options.BluemixInstance = core.StringPtr(bluemixInstance)
	return _options
}

// SetCorrelationID : Allow user to set CorrelationID
func (_options *GetPolicyOptions) SetCorrelationID(correlationID string) *GetPolicyOptions {
	_options.CorrelationID = core.StringPtr(correlationID)
	return _options
}

// SetXKmsKeyRing : Allow user to set XKmsKeyRing
func (_options *GetPolicyOptions) SetXKmsKeyRing(xKmsKeyRing string) *GetPolicyOptions {
	_options.XKmsKeyRing = core.StringPtr(xKmsKeyRing)
	return _options
}

// SetPolicy : Allow user to set Policy
func (_options *GetPolicyOptions) SetPolicy(policy string) *GetPolicyOptions {
	_options.Policy = core.StringPtr(policy)
	return _options
}

// SetHeaders : Allow user to set Headers
func (options *GetPolicyOptions) SetHeaders(param map[string]string) *GetPolicyOptions {
	options.Headers = param
	return options
}

// GetRegistrationsAllKeysOptions : The GetRegistrationsAllKeys options.
type GetRegistrationsAllKeysOptions struct {
	// The IBM Cloud instance ID that identifies your Key Protect service instance.
	BluemixInstance *string `json:"Bluemix-Instance" validate:"required"`

	// The v4 UUID used to correlate and track transactions.
	CorrelationID *string `json:"Correlation-Id,omitempty"`

	// The ID of the target key ring. If unspecified, all resources in the instance that the caller has access to will be
	// returned. When the header is specified, only resources within the specified key ring, that the caller has access to,
	// will be returned. The key ring ID of keys that are created without an `X-Kms-Key-Ring` header is: `default`.
	XKmsKeyRing *string `json:"X-Kms-Key-Ring,omitempty"`

	// Filters for resources that are associated with a specified [Cloud Resource Name](/docs/account?topic=account-crn)
	// (CRN) by using URL encoded wildcard characters (`*`). The parameter should contain all CRN segments and must be URL
	// encoded. If provided, the parameter should not contain (`*`) in the first eight segments. If this parameter is not
	// provided, registrations for all keys in the requested Key Protect instance are returned.
	UrlEncodedResourceCRNQuery *string `json:"urlEncodedResourceCRNQuery,omitempty"`

	// The number of registrations to retrieve. By default returns the first 200 registrations. To retrieve a different set
	// of registrations, use `limit` with `offset` to page through your available resources. The maximum value for `limit`
	// is 5,000.
	// **Usage:** If you have 20 registrations that are associated with a key, and you want to retrieve only the first 5
	// registrations, use `../registrations?limit=5`.
	Limit *int64 `json:"limit,omitempty"`

	// The number of registrations to skip. By specifying `offset`, you retrieve a subset of registrations that starts with
	// the `offset` value. Use `offset` with `limit` to page through your available resources.
	// **Usage:** If you have 100 registrations that are associated with a key, and you want to retrieve registrations 26
	// through 50, use `../registrations?offset=25&limit=25`.
	Offset *int64 `json:"offset,omitempty"`

	// Filters registrations based on the `preventKeyDeletion` property. You can use this query parameter to search for
	// registered cloud resources that are non-erasable due to a retention policy. This policy should only be set if a WORM
	// policy (https://www.ibm.com/docs/en/spectrum-scale/5.0.1?topic=ics-how-write-once-read-many-worm-storage-works) must
	// be satisfied. Do not set this policy by default.
	// **Usage:** To search for registered cloud resources that have a retention policy, use
	// `../registrations?preventKeyDeletion=true`.
	PreventKeyDeletion *bool `json:"preventKeyDeletion,omitempty"`

	// If set to `true`, returns `totalCount` in the response metadata for use with pagination. The `totalCount` value
	// returned specifies the total number of registrations that match the request, disregarding limit and offset.
	// **Usage:** To return the `totalCount` value for use with pagination, use `../registrations?totalCount=true`.
	TotalCount *bool `json:"totalCount,omitempty"`

	// Allows users to set headers on API requests.
	Headers map[string]string
}

// NewGetRegistrationsAllKeysOptions : Instantiate GetRegistrationsAllKeysOptions
func (*IbmKeyProtectApiV2) NewGetRegistrationsAllKeysOptions(bluemixInstance string) *GetRegistrationsAllKeysOptions {
	return &GetRegistrationsAllKeysOptions{
		BluemixInstance: core.StringPtr(bluemixInstance),
	}
}

// SetBluemixInstance : Allow user to set BluemixInstance
func (_options *GetRegistrationsAllKeysOptions) SetBluemixInstance(bluemixInstance string) *GetRegistrationsAllKeysOptions {
	_options.BluemixInstance = core.StringPtr(bluemixInstance)
	return _options
}

// SetCorrelationID : Allow user to set CorrelationID
func (_options *GetRegistrationsAllKeysOptions) SetCorrelationID(correlationID string) *GetRegistrationsAllKeysOptions {
	_options.CorrelationID = core.StringPtr(correlationID)
	return _options
}

// SetXKmsKeyRing : Allow user to set XKmsKeyRing
func (_options *GetRegistrationsAllKeysOptions) SetXKmsKeyRing(xKmsKeyRing string) *GetRegistrationsAllKeysOptions {
	_options.XKmsKeyRing = core.StringPtr(xKmsKeyRing)
	return _options
}

// SetUrlEncodedResourceCRNQuery : Allow user to set UrlEncodedResourceCRNQuery
func (_options *GetRegistrationsAllKeysOptions) SetUrlEncodedResourceCRNQuery(urlEncodedResourceCRNQuery string) *GetRegistrationsAllKeysOptions {
	_options.UrlEncodedResourceCRNQuery = core.StringPtr(urlEncodedResourceCRNQuery)
	return _options
}

// SetLimit : Allow user to set Limit
func (_options *GetRegistrationsAllKeysOptions) SetLimit(limit int64) *GetRegistrationsAllKeysOptions {
	_options.Limit = core.Int64Ptr(limit)
	return _options
}

// SetOffset : Allow user to set Offset
func (_options *GetRegistrationsAllKeysOptions) SetOffset(offset int64) *GetRegistrationsAllKeysOptions {
	_options.Offset = core.Int64Ptr(offset)
	return _options
}

// SetPreventKeyDeletion : Allow user to set PreventKeyDeletion
func (_options *GetRegistrationsAllKeysOptions) SetPreventKeyDeletion(preventKeyDeletion bool) *GetRegistrationsAllKeysOptions {
	_options.PreventKeyDeletion = core.BoolPtr(preventKeyDeletion)
	return _options
}

// SetTotalCount : Allow user to set TotalCount
func (_options *GetRegistrationsAllKeysOptions) SetTotalCount(totalCount bool) *GetRegistrationsAllKeysOptions {
	_options.TotalCount = core.BoolPtr(totalCount)
	return _options
}

// SetHeaders : Allow user to set Headers
func (options *GetRegistrationsAllKeysOptions) SetHeaders(param map[string]string) *GetRegistrationsAllKeysOptions {
	options.Headers = param
	return options
}

// GetRegistrationsOptions : The GetRegistrations options.
type GetRegistrationsOptions struct {
	// The v4 UUID that uniquely identifies the key.
	ID *string `json:"id" validate:"required,ne="`

	// The IBM Cloud instance ID that identifies your Key Protect service instance.
	BluemixInstance *string `json:"Bluemix-Instance" validate:"required"`

	// The v4 UUID used to correlate and track transactions.
	CorrelationID *string `json:"Correlation-Id,omitempty"`

	// The ID of the key ring that the specified key is a part of. When the header is not specified, Key Protect will
	// perform a key ring lookup. For a more optimized request, specify the key ring on every call. The key ring ID of keys
	// that are created without an `X-Kms-Key-Ring` header is: `default`.
	XKmsKeyRing *string `json:"X-Kms-Key-Ring,omitempty"`

	// The number of registrations to retrieve. By default returns the first 200 registrations. To retrieve a different set
	// of registrations, use `limit` with `offset` to page through your available resources. The maximum value for `limit`
	// is 5,000.
	// **Usage:** If you have 20 registrations that are associated with a key, and you want to retrieve only the first 5
	// registrations, use `../registrations?limit=5`.
	Limit *int64 `json:"limit,omitempty"`

	// The number of registrations to skip. By specifying `offset`, you retrieve a subset of registrations that starts with
	// the `offset` value. Use `offset` with `limit` to page through your available resources.
	// **Usage:** If you have 100 registrations that are associated with a key, and you want to retrieve registrations 26
	// through 50, use `../registrations?offset=25&limit=25`.
	Offset *int64 `json:"offset,omitempty"`

	// Filters for resources that are associated with a specified [Cloud Resource Name](/docs/account?topic=account-crn)
	// (CRN) by using URL encoded wildcard characters (`*`). The parameter should contain all CRN segments and must be URL
	// encoded. Supports a prefix search when you specify `*` on the last CRN segment.
	// **Usage:** To list registrations that are associated with all resources in `<service-instance>`, use a URL encoded
	// version of the following string:
	// `crn:v1:bluemix:public:<service-name>:<location>:a/<account>:<service-instance>:*:*`. To search for subresources,
	// use the following CRN format:
	// `crn:v1:bluemix:public:<service-name>:<location>:a/<account>:<service-instance>:<resource-type>:<resource>/<subresource>`.
	// For more examples, see [CRN query
	// examples](/docs/key-protect?topic=key-protect-view-protected-resources#crn-query-examples).
	UrlEncodedResourceCRNQuery *string `json:"urlEncodedResourceCRNQuery,omitempty"`

	// Filters registrations based on the `preventKeyDeletion` property. You can use this query parameter to search for
	// registered cloud resources that are non-erasable due to a retention policy. This policy should only be set if a WORM
	// policy (https://www.ibm.com/docs/en/spectrum-scale/5.0.1?topic=ics-how-write-once-read-many-worm-storage-works) must
	// be satisfied. Do not set this policy by default.
	// **Usage:** To search for registered cloud resources that have a retention policy, use
	// `../registrations?preventKeyDeletion=true`.
	PreventKeyDeletion *bool `json:"preventKeyDeletion,omitempty"`

	// If set to `true`, returns `totalCount` in the response metadata for use with pagination. The `totalCount` value
	// returned specifies the total number of registrations that match the request, disregarding limit and offset.
	// **Usage:** To return the `totalCount` value for use with pagination, use `../registrations?totalCount=true`.
	TotalCount *bool `json:"totalCount,omitempty"`

	// Allows users to set headers on API requests.
	Headers map[string]string
}

// NewGetRegistrationsOptions : Instantiate GetRegistrationsOptions
func (*IbmKeyProtectApiV2) NewGetRegistrationsOptions(id string, bluemixInstance string) *GetRegistrationsOptions {
	return &GetRegistrationsOptions{
		ID:              core.StringPtr(id),
		BluemixInstance: core.StringPtr(bluemixInstance),
	}
}

// SetID : Allow user to set ID
func (_options *GetRegistrationsOptions) SetID(id string) *GetRegistrationsOptions {
	_options.ID = core.StringPtr(id)
	return _options
}

// SetBluemixInstance : Allow user to set BluemixInstance
func (_options *GetRegistrationsOptions) SetBluemixInstance(bluemixInstance string) *GetRegistrationsOptions {
	_options.BluemixInstance = core.StringPtr(bluemixInstance)
	return _options
}

// SetCorrelationID : Allow user to set CorrelationID
func (_options *GetRegistrationsOptions) SetCorrelationID(correlationID string) *GetRegistrationsOptions {
	_options.CorrelationID = core.StringPtr(correlationID)
	return _options
}

// SetXKmsKeyRing : Allow user to set XKmsKeyRing
func (_options *GetRegistrationsOptions) SetXKmsKeyRing(xKmsKeyRing string) *GetRegistrationsOptions {
	_options.XKmsKeyRing = core.StringPtr(xKmsKeyRing)
	return _options
}

// SetLimit : Allow user to set Limit
func (_options *GetRegistrationsOptions) SetLimit(limit int64) *GetRegistrationsOptions {
	_options.Limit = core.Int64Ptr(limit)
	return _options
}

// SetOffset : Allow user to set Offset
func (_options *GetRegistrationsOptions) SetOffset(offset int64) *GetRegistrationsOptions {
	_options.Offset = core.Int64Ptr(offset)
	return _options
}

// SetUrlEncodedResourceCRNQuery : Allow user to set UrlEncodedResourceCRNQuery
func (_options *GetRegistrationsOptions) SetUrlEncodedResourceCRNQuery(urlEncodedResourceCRNQuery string) *GetRegistrationsOptions {
	_options.UrlEncodedResourceCRNQuery = core.StringPtr(urlEncodedResourceCRNQuery)
	return _options
}

// SetPreventKeyDeletion : Allow user to set PreventKeyDeletion
func (_options *GetRegistrationsOptions) SetPreventKeyDeletion(preventKeyDeletion bool) *GetRegistrationsOptions {
	_options.PreventKeyDeletion = core.BoolPtr(preventKeyDeletion)
	return _options
}

// SetTotalCount : Allow user to set TotalCount
func (_options *GetRegistrationsOptions) SetTotalCount(totalCount bool) *GetRegistrationsOptions {
	_options.TotalCount = core.BoolPtr(totalCount)
	return _options
}

// SetHeaders : Allow user to set Headers
func (options *GetRegistrationsOptions) SetHeaders(param map[string]string) *GetRegistrationsOptions {
	options.Headers = param
	return options
}

// ImportToken : Properties that are associated with import tokens.
type ImportToken struct {
	// The time in seconds from the creation of an import token that determines how long its associated public key remains
	// valid. The minimum value is `300` seconds (5 minutes), and the maximum value is `86400` (24 hours). The default
	// value is `600` (10 minutes).
	Expiration *float64 `json:"expiration,omitempty"`

	// The number of times that an import token can be retrieved within its expiration time before it is no longer
	// accessible.
	MaxAllowedRetrievals *float64 `json:"maxAllowedRetrievals,omitempty"`

	// The date the import token was created. The date format follows RFC 3339.
	CreationDate *strfmt.DateTime `json:"creationDate,omitempty"`

	// The date the import token expires. The date format follows RFC 3339.
	ExpirationDate *strfmt.DateTime `json:"expirationDate,omitempty"`

	// The number of retrievals that are available for the import token before it is no longer accessible.
	RemainingRetrievals *float64 `json:"remainingRetrievals,omitempty"`
}

// UnmarshalImportToken unmarshals an instance of ImportToken from the specified map of raw messages.
func UnmarshalImportToken(m map[string]json.RawMessage, result interface{}) (err error) {
	obj := new(ImportToken)
	err = core.UnmarshalPrimitive(m, "expiration", &obj.Expiration)
	if err != nil {
		err = core.SDKErrorf(err, "", "expiration-error", common.GetComponentInfo())
		return
	}
	err = core.UnmarshalPrimitive(m, "maxAllowedRetrievals", &obj.MaxAllowedRetrievals)
	if err != nil {
		err = core.SDKErrorf(err, "", "maxAllowedRetrievals-error", common.GetComponentInfo())
		return
	}
	err = core.UnmarshalPrimitive(m, "creationDate", &obj.CreationDate)
	if err != nil {
		err = core.SDKErrorf(err, "", "creationDate-error", common.GetComponentInfo())
		return
	}
	err = core.UnmarshalPrimitive(m, "expirationDate", &obj.ExpirationDate)
	if err != nil {
		err = core.SDKErrorf(err, "", "expirationDate-error", common.GetComponentInfo())
		return
	}
	err = core.UnmarshalPrimitive(m, "remainingRetrievals", &obj.RemainingRetrievals)
	if err != nil {
		err = core.SDKErrorf(err, "", "remainingRetrievals-error", common.GetComponentInfo())
		return
	}
	reflect.ValueOf(result).Elem().Set(reflect.ValueOf(obj))
	return
}

// InstancePolicyAllowedIPPolicyData : User defined metadata that is associated with the `allowedIP` instance policy type.
type InstancePolicyAllowedIPPolicyData struct {
	// If set to `true`, Key Protect enables the specified policy for your service instance. If set to `false`, Key Protect
	// disables the specified policy for your service instance, and the policy will no longer affect Key Protect actions.
	// **Note:** If a policy with attributes is disabled, all attributes are reset and are not retained.
	Enabled *bool `json:"enabled" validate:"required"`

	// Attributes of an `allowedIP` instance policy. Must be provided if the `enabled` field is `true`. Cannot be provided
	// if the `enabled` field is `false`.
	Attributes *InstancePolicyAllowedIPPolicyDataAttributes `json:"attributes,omitempty"`
}

// NewInstancePolicyAllowedIPPolicyData : Instantiate InstancePolicyAllowedIPPolicyData (Generic Model Constructor)
func (*IbmKeyProtectApiV2) NewInstancePolicyAllowedIPPolicyData(enabled bool) (_model *InstancePolicyAllowedIPPolicyData, err error) {
	_model = &InstancePolicyAllowedIPPolicyData{
		Enabled: core.BoolPtr(enabled),
	}
	err = core.ValidateStruct(_model, "required parameters")
	if err != nil {
		err = core.SDKErrorf(err, "", "model-missing-required", common.GetComponentInfo())
	}
	return
}

// UnmarshalInstancePolicyAllowedIPPolicyData unmarshals an instance of InstancePolicyAllowedIPPolicyData from the specified map of raw messages.
func UnmarshalInstancePolicyAllowedIPPolicyData(m map[string]json.RawMessage, result interface{}) (err error) {
	obj := new(InstancePolicyAllowedIPPolicyData)
	err = core.UnmarshalPrimitive(m, "enabled", &obj.Enabled)
	if err != nil {
		err = core.SDKErrorf(err, "", "enabled-error", common.GetComponentInfo())
		return
	}
	err = core.UnmarshalModel(m, "attributes", &obj.Attributes, UnmarshalInstancePolicyAllowedIPPolicyDataAttributes)
	if err != nil {
		err = core.SDKErrorf(err, "", "attributes-error", common.GetComponentInfo())
		return
	}
	reflect.ValueOf(result).Elem().Set(reflect.ValueOf(obj))
	return
}

// InstancePolicyAllowedIPPolicyDataAttributes : Attributes of an `allowedIP` instance policy. Must be provided if the `enabled` field is `true`. Cannot be provided
// if the `enabled` field is `false`.
type InstancePolicyAllowedIPPolicyDataAttributes struct {
	// A string array of IPv4 or IPv6 CIDR notated subnets that are authorized to interact with the instance. If both
	// `allowedNetwork` and `allowedIP` policies are set, only traffic aligning with both the `allowed_network` allowed
	// network policy attribute and the `allowed_ip` allowed IP policy attribute will be allowed. IPv4 and iIP6 addresses
	// are accepted for public endpoints. Only the IPv4 private network gateway addresses from the array will be authorized
	// to access your instance via private endpoint.
	// **Important:** Once set, accessing your instance may require additional steps. For more information, see [Accessing
	// an instance via public
	// endpoint](/docs/key-protect?topic=key-protect-manage-allowed-ip#access-allowed-ip-public-endpoint) and [Accessing an
	// instance via private
	// endpoint](/docs/key-protect?topic=key-protect-manage-allowed-ip#access-allowed-ip-private-endpoint) for more
	// details.
	// **Note:** An allowed IP policy does not affect requests from other IBM Cloud services.
	AllowedIp []string `json:"allowed_ip,omitempty"`
}

// UnmarshalInstancePolicyAllowedIPPolicyDataAttributes unmarshals an instance of InstancePolicyAllowedIPPolicyDataAttributes from the specified map of raw messages.
func UnmarshalInstancePolicyAllowedIPPolicyDataAttributes(m map[string]json.RawMessage, result interface{}) (err error) {
	obj := new(InstancePolicyAllowedIPPolicyDataAttributes)
	err = core.UnmarshalPrimitive(m, "allowed_ip", &obj.AllowedIp)
	if err != nil {
		err = core.SDKErrorf(err, "", "allowed_ip-error", common.GetComponentInfo())
		return
	}
	reflect.ValueOf(result).Elem().Set(reflect.ValueOf(obj))
	return
}

// InstancePolicyAllowedNetworkPolicyData : User defined metadata that is associated with the `allowedNetwork` instance policy type.
type InstancePolicyAllowedNetworkPolicyData struct {
	// If set to `true`, Key Protect enables the specified policy for your service instance. If set to `false`, Key Protect
	// disables the specified policy for your service instance, and the policy will no longer affect Key Protect actions.
	// **Note:** If a policy with attributes is disabled, all attributes are reset and are not retained.
	Enabled *bool `json:"enabled" validate:"required"`

	// Attributes of an `allowedNetwork` instance policy. Must be provided if the `enabled` field is `true`. Cannot be
	// provided if the `enabled` field is `false`.
	Attributes *InstancePolicyAllowedNetworkPolicyDataAttributes `json:"attributes,omitempty"`
}

// NewInstancePolicyAllowedNetworkPolicyData : Instantiate InstancePolicyAllowedNetworkPolicyData (Generic Model Constructor)
func (*IbmKeyProtectApiV2) NewInstancePolicyAllowedNetworkPolicyData(enabled bool) (_model *InstancePolicyAllowedNetworkPolicyData, err error) {
	_model = &InstancePolicyAllowedNetworkPolicyData{
		Enabled: core.BoolPtr(enabled),
	}
	err = core.ValidateStruct(_model, "required parameters")
	if err != nil {
		err = core.SDKErrorf(err, "", "model-missing-required", common.GetComponentInfo())
	}
	return
}

// UnmarshalInstancePolicyAllowedNetworkPolicyData unmarshals an instance of InstancePolicyAllowedNetworkPolicyData from the specified map of raw messages.
func UnmarshalInstancePolicyAllowedNetworkPolicyData(m map[string]json.RawMessage, result interface{}) (err error) {
	obj := new(InstancePolicyAllowedNetworkPolicyData)
	err = core.UnmarshalPrimitive(m, "enabled", &obj.Enabled)
	if err != nil {
		err = core.SDKErrorf(err, "", "enabled-error", common.GetComponentInfo())
		return
	}
	err = core.UnmarshalModel(m, "attributes", &obj.Attributes, UnmarshalInstancePolicyAllowedNetworkPolicyDataAttributes)
	if err != nil {
		err = core.SDKErrorf(err, "", "attributes-error", common.GetComponentInfo())
		return
	}
	reflect.ValueOf(result).Elem().Set(reflect.ValueOf(obj))
	return
}

// InstancePolicyAllowedNetworkPolicyDataAttributes : Attributes of an `allowedNetwork` instance policy. Must be provided if the `enabled` field is `true`. Cannot be
// provided if the `enabled` field is `false`.
type InstancePolicyAllowedNetworkPolicyDataAttributes struct {
	// If set to `public-and-private`, Key Protect allows the instance to be accessible through public and private
	// endpoints. If set to `private-only`, Key Protect restricts the instance to only be accessible through a private
	// endpoint.
	AllowedNetwork *string `json:"allowed_network" validate:"required"`
}

// Constants associated with the InstancePolicyAllowedNetworkPolicyDataAttributes.AllowedNetwork property.
// If set to `public-and-private`, Key Protect allows the instance to be accessible through public and private
// endpoints. If set to `private-only`, Key Protect restricts the instance to only be accessible through a private
// endpoint.
const (
	InstancePolicyAllowedNetworkPolicyDataAttributes_AllowedNetwork_PrivateOnly      = "private-only"
	InstancePolicyAllowedNetworkPolicyDataAttributes_AllowedNetwork_PublicAndPrivate = "public-and-private"
)

// NewInstancePolicyAllowedNetworkPolicyDataAttributes : Instantiate InstancePolicyAllowedNetworkPolicyDataAttributes (Generic Model Constructor)
func (*IbmKeyProtectApiV2) NewInstancePolicyAllowedNetworkPolicyDataAttributes(allowedNetwork string) (_model *InstancePolicyAllowedNetworkPolicyDataAttributes, err error) {
	_model = &InstancePolicyAllowedNetworkPolicyDataAttributes{
		AllowedNetwork: core.StringPtr(allowedNetwork),
	}
	err = core.ValidateStruct(_model, "required parameters")
	if err != nil {
		err = core.SDKErrorf(err, "", "model-missing-required", common.GetComponentInfo())
	}
	return
}

// UnmarshalInstancePolicyAllowedNetworkPolicyDataAttributes unmarshals an instance of InstancePolicyAllowedNetworkPolicyDataAttributes from the specified map of raw messages.
func UnmarshalInstancePolicyAllowedNetworkPolicyDataAttributes(m map[string]json.RawMessage, result interface{}) (err error) {
	obj := new(InstancePolicyAllowedNetworkPolicyDataAttributes)
	err = core.UnmarshalPrimitive(m, "allowed_network", &obj.AllowedNetwork)
	if err != nil {
		err = core.SDKErrorf(err, "", "allowed_network-error", common.GetComponentInfo())
		return
	}
	reflect.ValueOf(result).Elem().Set(reflect.ValueOf(obj))
	return
}

// InstancePolicyKeyCreateImportAccessPolicyData : User defined metadata that is associated with the `keyCreateImportAccess` instance policy type.
type InstancePolicyKeyCreateImportAccessPolicyData struct {
	// If set to `true`, Key Protect enables the specified policy for your service instance. If set to `false`, Key Protect
	// disables the specified policy for your service instance, and the policy will no longer affect Key Protect actions.
	// **Note:** If a policy with attributes is disabled, all attributes are reset and are not retained.
	Enabled *bool `json:"enabled" validate:"required"`

	// Attributes of a `keyCreateImportAccess` instance policy. Must be provided if the `enabled` field is `true`. Cannot
	// be provided if the `enabled` field is `false`.
	Attributes *InstancePolicyKeyCreateImportAccessPolicyDataAttributes `json:"attributes,omitempty"`
}

// NewInstancePolicyKeyCreateImportAccessPolicyData : Instantiate InstancePolicyKeyCreateImportAccessPolicyData (Generic Model Constructor)
func (*IbmKeyProtectApiV2) NewInstancePolicyKeyCreateImportAccessPolicyData(enabled bool) (_model *InstancePolicyKeyCreateImportAccessPolicyData, err error) {
	_model = &InstancePolicyKeyCreateImportAccessPolicyData{
		Enabled: core.BoolPtr(enabled),
	}
	err = core.ValidateStruct(_model, "required parameters")
	if err != nil {
		err = core.SDKErrorf(err, "", "model-missing-required", common.GetComponentInfo())
	}
	return
}

// UnmarshalInstancePolicyKeyCreateImportAccessPolicyData unmarshals an instance of InstancePolicyKeyCreateImportAccessPolicyData from the specified map of raw messages.
func UnmarshalInstancePolicyKeyCreateImportAccessPolicyData(m map[string]json.RawMessage, result interface{}) (err error) {
	obj := new(InstancePolicyKeyCreateImportAccessPolicyData)
	err = core.UnmarshalPrimitive(m, "enabled", &obj.Enabled)
	if err != nil {
		err = core.SDKErrorf(err, "", "enabled-error", common.GetComponentInfo())
		return
	}
	err = core.UnmarshalModel(m, "attributes", &obj.Attributes, UnmarshalInstancePolicyKeyCreateImportAccessPolicyDataAttributes)
	if err != nil {
		err = core.SDKErrorf(err, "", "attributes-error", common.GetComponentInfo())
		return
	}
	reflect.ValueOf(result).Elem().Set(reflect.ValueOf(obj))
	return
}

// InstancePolicyKeyCreateImportAccessPolicyDataAttributes : Attributes of a `keyCreateImportAccess` instance policy. Must be provided if the `enabled` field is `true`. Cannot be
// provided if the `enabled` field is `false`.
type InstancePolicyKeyCreateImportAccessPolicyDataAttributes struct {
	// If set to `false`, the service prevents you or any authorized users from using Key Protect to create root keys in
	// the specified service instance. If set to `true`, Key Protect allows you or any authorized users to create root keys
	// in the instance.
	// **Note:** If omitted, `POST /instance/policies` will set this attribute to the default value (`true`).
	CreateRootKey *bool `json:"create_root_key,omitempty"`

	// If set to `false`, the service prevents you or any authorized users from using Key Protect to create standard keys
	// in the specified service instance. If set to `true`, Key Protect allows you or any authorized users to create
	// standard keys in the instance.
	// **Note:** If omitted, `POST /instance/policies` will set this attribute to the default value (`true`).
	CreateStandardKey *bool `json:"create_standard_key,omitempty"`

	// If set to `false`, the service prevents you or any authorized users from importing root keys into the specified
	// service instance. If set to `true`, Key Protect allows you or any authorized users to import root keys into the
	// instance.
	// **Note:** If omitted, `POST /instance/policies` will set this attribute to the default value (`true`).
	ImportRootKey *bool `json:"import_root_key,omitempty"`

	// If set to `false`, the service prevents you or any authorized users from importing standard keys into the specified
	// service instance. If set to `true`, Key Protect allows you or any authorized users to import standard keys into the
	// instance.
	// **Note:** If omitted, `POST /instance/policies` will set this attribute to the default value (`true`).
	ImportStandardKey *bool `json:"import_standard_key,omitempty"`

	// If set to `true`, the service prevents you or any authorized users from importing key material into the specified
	// service instance without using an import token. If set to `false`, Key Protect allows you or any authorized users to
	// import key material into the instance without the use of an import token.
	// **Note:** If omitted, `POST /instance/policies` will set this attribute to the default value (`false`).
	EnforceToken *bool `json:"enforce_token,omitempty"`
}

// UnmarshalInstancePolicyKeyCreateImportAccessPolicyDataAttributes unmarshals an instance of InstancePolicyKeyCreateImportAccessPolicyDataAttributes from the specified map of raw messages.
func UnmarshalInstancePolicyKeyCreateImportAccessPolicyDataAttributes(m map[string]json.RawMessage, result interface{}) (err error) {
	obj := new(InstancePolicyKeyCreateImportAccessPolicyDataAttributes)
	err = core.UnmarshalPrimitive(m, "create_root_key", &obj.CreateRootKey)
	if err != nil {
		err = core.SDKErrorf(err, "", "create_root_key-error", common.GetComponentInfo())
		return
	}
	err = core.UnmarshalPrimitive(m, "create_standard_key", &obj.CreateStandardKey)
	if err != nil {
		err = core.SDKErrorf(err, "", "create_standard_key-error", common.GetComponentInfo())
		return
	}
	err = core.UnmarshalPrimitive(m, "import_root_key", &obj.ImportRootKey)
	if err != nil {
		err = core.SDKErrorf(err, "", "import_root_key-error", common.GetComponentInfo())
		return
	}
	err = core.UnmarshalPrimitive(m, "import_standard_key", &obj.ImportStandardKey)
	if err != nil {
		err = core.SDKErrorf(err, "", "import_standard_key-error", common.GetComponentInfo())
		return
	}
	err = core.UnmarshalPrimitive(m, "enforce_token", &obj.EnforceToken)
	if err != nil {
		err = core.SDKErrorf(err, "", "enforce_token-error", common.GetComponentInfo())
		return
	}
	reflect.ValueOf(result).Elem().Set(reflect.ValueOf(obj))
	return
}

// InstancePolicyProperties : User defined metadata that is associated with any instance policy.
type InstancePolicyProperties struct {
	// If set to `true`, Key Protect enables the specified policy for your service instance. If set to `false`, Key Protect
	// disables the specified policy for your service instance, and the policy will no longer affect Key Protect actions.
	// **Note:** If a policy with attributes is disabled, all attributes are reset and are not retained.
	Enabled *bool `json:"enabled" validate:"required"`

	// Attributes associated with any instance policy type.
	Attributes *InstancePolicyPropertiesAttributes `json:"attributes,omitempty"`
}

// UnmarshalInstancePolicyProperties unmarshals an instance of InstancePolicyProperties from the specified map of raw messages.
func UnmarshalInstancePolicyProperties(m map[string]json.RawMessage, result interface{}) (err error) {
	obj := new(InstancePolicyProperties)
	err = core.UnmarshalPrimitive(m, "enabled", &obj.Enabled)
	if err != nil {
		err = core.SDKErrorf(err, "", "enabled-error", common.GetComponentInfo())
		return
	}
	err = core.UnmarshalModel(m, "attributes", &obj.Attributes, UnmarshalInstancePolicyPropertiesAttributes)
	if err != nil {
		err = core.SDKErrorf(err, "", "attributes-error", common.GetComponentInfo())
		return
	}
	reflect.ValueOf(result).Elem().Set(reflect.ValueOf(obj))
	return
}

// InstancePolicyPropertiesAttributes : Attributes associated with any instance policy type.
type InstancePolicyPropertiesAttributes struct {
	// If set to `public-and-private`, Key Protect allows the instance to be accessible through public and private
	// endpoints. If set to `private-only`, Key Protect restricts the instance to only be accessible through a private
	// endpoint.
	AllowedNetwork *string `json:"allowed_network,omitempty"`

	// A string array of IPv4 or IPv6 CIDR notated subnets that are authorized to interact with the instance. If both
	// `allowedNetwork` and `allowedIP` policies are set, only traffic aligning with both the `allowed_network` allowed
	// network policy attribute and the `allowed_ip` allowed IP policy attribute will be allowed. IPv4 and iIP6 addresses
	// are accepted for public endpoints. Only the IPv4 private network gateway addresses from the array will be authorized
	// to access your instance via private endpoint.
	// **Important:** Once set, accessing your instance may require additional steps. For more information, see [Accessing
	// an instance via public
	// endpoint](/docs/key-protect?topic=key-protect-manage-allowed-ip#access-allowed-ip-public-endpoint) and [Accessing an
	// instance via private
	// endpoint](/docs/key-protect?topic=key-protect-manage-allowed-ip#access-allowed-ip-private-endpoint) for more
	// details.
	// **Note:** An allowed IP policy does not affect requests from other IBM Cloud services.
	AllowedIp []string `json:"allowed_ip,omitempty"`

	// If set to `false`, the service prevents you or any authorized users from using Key Protect to create root keys in
	// the specified service instance. If set to `true`, Key Protect allows you or any authorized users to create root keys
	// in the instance.
	// **Note:** If omitted, `POST /instance/policies` will set this attribute to the default value (`true`).
	CreateRootKey *bool `json:"create_root_key,omitempty"`

	// If set to `false`, the service prevents you or any authorized users from using Key Protect to create standard keys
	// in the specified service instance. If set to `true`, Key Protect allows you or any authorized users to create
	// standard keys in the instance.
	// **Note:** If omitted, `POST /instance/policies` will set this attribute to the default value (`true`).
	CreateStandardKey *bool `json:"create_standard_key,omitempty"`

	// If set to `false`, the service prevents you or any authorized users from importing root keys into the specified
	// service instance. If set to `true`, Key Protect allows you or any authorized users to import root keys into the
	// instance.
	// **Note:** If omitted, `POST /instance/policies` will set this attribute to the default value (`true`).
	ImportRootKey *bool `json:"import_root_key,omitempty"`

	// If set to `false`, the service prevents you or any authorized users from importing standard keys into the specified
	// service instance. If set to `true`, Key Protect allows you or any authorized users to import standard keys into the
	// instance.
	// **Note:** If omitted, `POST /instance/policies` will set this attribute to the default value (`true`).
	ImportStandardKey *bool `json:"import_standard_key,omitempty"`

	// If set to `true`, the service prevents you or any authorized users from importing key material into the specified
	// service instance without using an import token. If set to `false`, Key Protect allows you or any authorized users to
	// import key material into the instance without the use of an import token.
	// **Note:** If omitted, `POST /instance/policies` will set this attribute to the default value (`false`).
	EnforceToken *bool `json:"enforce_token,omitempty"`

	// Specifies the key rotation time interval in approximate months, where a month is equivalent to 30 days. A minimum of
	// 1 and a maximum of 12 can be set.
	IntervalMonth *int64 `json:"interval_month,omitempty"`
}

// Constants associated with the InstancePolicyPropertiesAttributes.AllowedNetwork property.
// If set to `public-and-private`, Key Protect allows the instance to be accessible through public and private
// endpoints. If set to `private-only`, Key Protect restricts the instance to only be accessible through a private
// endpoint.
const (
	InstancePolicyPropertiesAttributes_AllowedNetwork_PrivateOnly      = "private-only"
	InstancePolicyPropertiesAttributes_AllowedNetwork_PublicAndPrivate = "public-and-private"
)

// UnmarshalInstancePolicyPropertiesAttributes unmarshals an instance of InstancePolicyPropertiesAttributes from the specified map of raw messages.
func UnmarshalInstancePolicyPropertiesAttributes(m map[string]json.RawMessage, result interface{}) (err error) {
	obj := new(InstancePolicyPropertiesAttributes)
	err = core.UnmarshalPrimitive(m, "allowed_network", &obj.AllowedNetwork)
	if err != nil {
		err = core.SDKErrorf(err, "", "allowed_network-error", common.GetComponentInfo())
		return
	}
	err = core.UnmarshalPrimitive(m, "allowed_ip", &obj.AllowedIp)
	if err != nil {
		err = core.SDKErrorf(err, "", "allowed_ip-error", common.GetComponentInfo())
		return
	}
	err = core.UnmarshalPrimitive(m, "create_root_key", &obj.CreateRootKey)
	if err != nil {
		err = core.SDKErrorf(err, "", "create_root_key-error", common.GetComponentInfo())
		return
	}
	err = core.UnmarshalPrimitive(m, "create_standard_key", &obj.CreateStandardKey)
	if err != nil {
		err = core.SDKErrorf(err, "", "create_standard_key-error", common.GetComponentInfo())
		return
	}
	err = core.UnmarshalPrimitive(m, "import_root_key", &obj.ImportRootKey)
	if err != nil {
		err = core.SDKErrorf(err, "", "import_root_key-error", common.GetComponentInfo())
		return
	}
	err = core.UnmarshalPrimitive(m, "import_standard_key", &obj.ImportStandardKey)
	if err != nil {
		err = core.SDKErrorf(err, "", "import_standard_key-error", common.GetComponentInfo())
		return
	}
	err = core.UnmarshalPrimitive(m, "enforce_token", &obj.EnforceToken)
	if err != nil {
		err = core.SDKErrorf(err, "", "enforce_token-error", common.GetComponentInfo())
		return
	}
	err = core.UnmarshalPrimitive(m, "interval_month", &obj.IntervalMonth)
	if err != nil {
		err = core.SDKErrorf(err, "", "interval_month-error", common.GetComponentInfo())
		return
	}
	reflect.ValueOf(result).Elem().Set(reflect.ValueOf(obj))
	return
}

// InstancePolicyResource : InstancePolicyResource struct
type InstancePolicyResource struct {
	// The date the policy was created. The date format follows RFC 3339.
	CreationDate *strfmt.DateTime `json:"creationDate,omitempty"`

	// The unique identifier for the resource that created the policy.
	CreatedBy *string `json:"createdBy,omitempty"`

	// The unique identifier for the resource that updated the policy.
	UpdatedBy *string `json:"updatedBy,omitempty"`

	// Updates when the policy is replaced or modified. The date format follows RFC 3339.
	LastUpdated *strfmt.DateTime `json:"lastUpdated,omitempty"`

	// The type of policy to be retrieved.
	PolicyType *string `json:"policy_type" validate:"required"`

	// User defined metadata that is associated with any instance policy.
	PolicyData *InstancePolicyProperties `json:"policy_data" validate:"required"`
}

// UnmarshalInstancePolicyResource unmarshals an instance of InstancePolicyResource from the specified map of raw messages.
func UnmarshalInstancePolicyResource(m map[string]json.RawMessage, result interface{}) (err error) {
	obj := new(InstancePolicyResource)
	err = core.UnmarshalPrimitive(m, "creationDate", &obj.CreationDate)
	if err != nil {
		err = core.SDKErrorf(err, "", "creationDate-error", common.GetComponentInfo())
		return
	}
	err = core.UnmarshalPrimitive(m, "createdBy", &obj.CreatedBy)
	if err != nil {
		err = core.SDKErrorf(err, "", "createdBy-error", common.GetComponentInfo())
		return
	}
	err = core.UnmarshalPrimitive(m, "updatedBy", &obj.UpdatedBy)
	if err != nil {
		err = core.SDKErrorf(err, "", "updatedBy-error", common.GetComponentInfo())
		return
	}
	err = core.UnmarshalPrimitive(m, "lastUpdated", &obj.LastUpdated)
	if err != nil {
		err = core.SDKErrorf(err, "", "lastUpdated-error", common.GetComponentInfo())
		return
	}
	err = core.UnmarshalPrimitive(m, "policy_type", &obj.PolicyType)
	if err != nil {
		err = core.SDKErrorf(err, "", "policy_type-error", common.GetComponentInfo())
		return
	}
	err = core.UnmarshalModel(m, "policy_data", &obj.PolicyData, UnmarshalInstancePolicyProperties)
	if err != nil {
		err = core.SDKErrorf(err, "", "policy_data-error", common.GetComponentInfo())
		return
	}
	reflect.ValueOf(result).Elem().Set(reflect.ValueOf(obj))
	return
}

// InstancePolicyRotationPolicyData : User defined metadata that is associated with the `rotation` instance policy type.
type InstancePolicyRotationPolicyData struct {
	// If set to `true`, Key Protect enables the specified policy for your service instance. If set to `false`, Key Protect
	// disables the specified policy for your service instance, and the policy will no longer affect Key Protect actions.
	// **Note:** If a policy with attributes is disabled, all attributes are reset and are not retained.
	Enabled *bool `json:"enabled" validate:"required"`

	// Attributes of a `rotation` instance policy. Must be provided if the `enabled` field is `true`. Cannot be provided if
	// the `enabled` field is `false`.
	Attributes *InstancePolicyRotationPolicyDataAttributes `json:"attributes,omitempty"`
}

// NewInstancePolicyRotationPolicyData : Instantiate InstancePolicyRotationPolicyData (Generic Model Constructor)
func (*IbmKeyProtectApiV2) NewInstancePolicyRotationPolicyData(enabled bool) (_model *InstancePolicyRotationPolicyData, err error) {
	_model = &InstancePolicyRotationPolicyData{
		Enabled: core.BoolPtr(enabled),
	}
	err = core.ValidateStruct(_model, "required parameters")
	if err != nil {
		err = core.SDKErrorf(err, "", "model-missing-required", common.GetComponentInfo())
	}
	return
}

// UnmarshalInstancePolicyRotationPolicyData unmarshals an instance of InstancePolicyRotationPolicyData from the specified map of raw messages.
func UnmarshalInstancePolicyRotationPolicyData(m map[string]json.RawMessage, result interface{}) (err error) {
	obj := new(InstancePolicyRotationPolicyData)
	err = core.UnmarshalPrimitive(m, "enabled", &obj.Enabled)
	if err != nil {
		err = core.SDKErrorf(err, "", "enabled-error", common.GetComponentInfo())
		return
	}
	err = core.UnmarshalModel(m, "attributes", &obj.Attributes, UnmarshalInstancePolicyRotationPolicyDataAttributes)
	if err != nil {
		err = core.SDKErrorf(err, "", "attributes-error", common.GetComponentInfo())
		return
	}
	reflect.ValueOf(result).Elem().Set(reflect.ValueOf(obj))
	return
}

// InstancePolicyRotationPolicyDataAttributes : Attributes of a `rotation` instance policy. Must be provided if the `enabled` field is `true`. Cannot be provided if
// the `enabled` field is `false`.
type InstancePolicyRotationPolicyDataAttributes struct {
	// Specifies the key rotation time interval in approximate months, where a month is equivalent to 30 days. A minimum of
	// 1 and a maximum of 12 can be set.
	IntervalMonth *int64 `json:"interval_month,omitempty"`
}

// UnmarshalInstancePolicyRotationPolicyDataAttributes unmarshals an instance of InstancePolicyRotationPolicyDataAttributes from the specified map of raw messages.
func UnmarshalInstancePolicyRotationPolicyDataAttributes(m map[string]json.RawMessage, result interface{}) (err error) {
	obj := new(InstancePolicyRotationPolicyDataAttributes)
	err = core.UnmarshalPrimitive(m, "interval_month", &obj.IntervalMonth)
	if err != nil {
		err = core.SDKErrorf(err, "", "interval_month-error", common.GetComponentInfo())
		return
	}
	reflect.ValueOf(result).Elem().Set(reflect.ValueOf(obj))
	return
}

// KMIPAdapter : Properties applicable to all KMIP adapter resources.
type KMIPAdapter struct {
	// The v4 UUID that uniquely identifies this KMIP adapter.
	ID *string `json:"id" validate:"required"`

	// A human-readable name of the KMIP adapter unique within the kms instance. If one is not specified, one will be
	// autogenerated of the format `kmip_adapter_<random_string>`. To protect your privacy do not use personal data, such
	// as your name or location, as a name for your KMIP adapter. The name must be alphanumeric and cannot contain spaces
	// or special characters other than `-` or `_`. The name cannot be a UUID.
	Name *string `json:"name" validate:"required"`

	// The date the KMIP adapter was created. The date format follows RFC 3339.
	CreatedAt *strfmt.DateTime `json:"created_at" validate:"required"`

	// The unique identifier of the user that created the KMIP adapter.
	CreatedBy *string `json:"created_by" validate:"required"`

	// The date the KMIP adapter was last modified, either by creation or by modification of adapter subresources. The date
	// format follows RFC 3339.
	UpdatedAt *strfmt.DateTime `json:"updated_at" validate:"required"`

	// The unique identifier of the user that updated the KMIP adapter.
	UpdatedBy *string `json:"updated_by" validate:"required"`

	// The profile of KMIP adapter.
	Profile *string `json:"profile" validate:"required"`

	// The optional description of the KMIP adapter. The maximum length is 240 characters. To protect your privacy, do not
	// use personal data, such as your name or location, as a description for your KMIP adapter.
	Description *string `json:"description,omitempty"`

	// The data specific to the KMIP Adapter profile. This is a required field for profile `native_1.0`.
	ProfileData KMIPProfileDataBodyIntf `json:"profile_data,omitempty"`
}

// Constants associated with the KMIPAdapter.Profile property.
// The profile of KMIP adapter.
const (
	KMIPAdapter_Profile_Native10 = "native_1.0"
)

// UnmarshalKMIPAdapter unmarshals an instance of KMIPAdapter from the specified map of raw messages.
func UnmarshalKMIPAdapter(m map[string]json.RawMessage, result interface{}) (err error) {
	obj := new(KMIPAdapter)
	err = core.UnmarshalPrimitive(m, "id", &obj.ID)
	if err != nil {
		err = core.SDKErrorf(err, "", "id-error", common.GetComponentInfo())
		return
	}
	err = core.UnmarshalPrimitive(m, "name", &obj.Name)
	if err != nil {
		err = core.SDKErrorf(err, "", "name-error", common.GetComponentInfo())
		return
	}
	err = core.UnmarshalPrimitive(m, "created_at", &obj.CreatedAt)
	if err != nil {
		err = core.SDKErrorf(err, "", "created_at-error", common.GetComponentInfo())
		return
	}
	err = core.UnmarshalPrimitive(m, "created_by", &obj.CreatedBy)
	if err != nil {
		err = core.SDKErrorf(err, "", "created_by-error", common.GetComponentInfo())
		return
	}
	err = core.UnmarshalPrimitive(m, "updated_at", &obj.UpdatedAt)
	if err != nil {
		err = core.SDKErrorf(err, "", "updated_at-error", common.GetComponentInfo())
		return
	}
	err = core.UnmarshalPrimitive(m, "updated_by", &obj.UpdatedBy)
	if err != nil {
		err = core.SDKErrorf(err, "", "updated_by-error", common.GetComponentInfo())
		return
	}
	err = core.UnmarshalPrimitive(m, "profile", &obj.Profile)
	if err != nil {
		err = core.SDKErrorf(err, "", "profile-error", common.GetComponentInfo())
		return
	}
	err = core.UnmarshalPrimitive(m, "description", &obj.Description)
	if err != nil {
		err = core.SDKErrorf(err, "", "description-error", common.GetComponentInfo())
		return
	}
	err = core.UnmarshalModel(m, "profile_data", &obj.ProfileData, UnmarshalKMIPProfileDataBody)
	if err != nil {
		err = core.SDKErrorf(err, "", "profile_data-error", common.GetComponentInfo())
		return
	}
	reflect.ValueOf(result).Elem().Set(reflect.ValueOf(obj))
	return
}

// KMIPClientCertificate : Properties of a client certificate.
type KMIPClientCertificate struct {
	// A human-readable name that uniquely identifies a certificate within the given adapter. If one is not specified, one
	// will be autogenerated of the format `kmip_cert_<random_string>`. To protect your privacy do not use personal data,
	// such as your name or location, as a name for your client certificate. The name must be alphanumeric and cannot
	// contain spaces or special characters other than `-` or `_`. The name cannot be a UUID.
	Name *string `json:"name" validate:"required"`

	// The v4 UUID that uniquely identifies this certificate resource.
	ID *string `json:"id" validate:"required"`

	// The date this certificate resource was created on the KMIP Adapter. The date format follows RFC 3339.
	CreatedAt *strfmt.DateTime `json:"created_at" validate:"required"`

	// The IAM id that created the certificate resource.
	CreatedBy *string `json:"created_by" validate:"required"`

	// The client certificate to be associated with the KMIP Adapter. It should explicitly have the BEGIN CERTIFICATE and
	// END CERTIFICATE tags.
	Certificate *string `json:"certificate" validate:"required"`
}

// UnmarshalKMIPClientCertificate unmarshals an instance of KMIPClientCertificate from the specified map of raw messages.
func UnmarshalKMIPClientCertificate(m map[string]json.RawMessage, result interface{}) (err error) {
	obj := new(KMIPClientCertificate)
	err = core.UnmarshalPrimitive(m, "name", &obj.Name)
	if err != nil {
		err = core.SDKErrorf(err, "", "name-error", common.GetComponentInfo())
		return
	}
	err = core.UnmarshalPrimitive(m, "id", &obj.ID)
	if err != nil {
		err = core.SDKErrorf(err, "", "id-error", common.GetComponentInfo())
		return
	}
	err = core.UnmarshalPrimitive(m, "created_at", &obj.CreatedAt)
	if err != nil {
		err = core.SDKErrorf(err, "", "created_at-error", common.GetComponentInfo())
		return
	}
	err = core.UnmarshalPrimitive(m, "created_by", &obj.CreatedBy)
	if err != nil {
		err = core.SDKErrorf(err, "", "created_by-error", common.GetComponentInfo())
		return
	}
	err = core.UnmarshalPrimitive(m, "certificate", &obj.Certificate)
	if err != nil {
		err = core.SDKErrorf(err, "", "certificate-error", common.GetComponentInfo())
		return
	}
	reflect.ValueOf(result).Elem().Set(reflect.ValueOf(obj))
	return
}

// KMIPClientPartialCertificate : Partial properties of a client certificate.
type KMIPClientPartialCertificate struct {
	// A human-readable name that uniquely identifies a certificate within the given adapter. If one is not specified, one
	// will be autogenerated of the format `kmip_cert_<random_string>`. To protect your privacy do not use personal data,
	// such as your name or location, as a name for your client certificate. The name must be alphanumeric and cannot
	// contain spaces or special characters other than `-` or `_`. The name cannot be a UUID.
	Name *string `json:"name" validate:"required"`

	// The v4 UUID that uniquely identifies this certificate resource.
	ID *string `json:"id" validate:"required"`

	// The date this certificate resource was created on the KMIP Adapter. The date format follows RFC 3339.
	CreatedAt *strfmt.DateTime `json:"created_at" validate:"required"`

	// The IAM id that created the certificate resource.
	CreatedBy *string `json:"created_by" validate:"required"`
}

// UnmarshalKMIPClientPartialCertificate unmarshals an instance of KMIPClientPartialCertificate from the specified map of raw messages.
func UnmarshalKMIPClientPartialCertificate(m map[string]json.RawMessage, result interface{}) (err error) {
	obj := new(KMIPClientPartialCertificate)
	err = core.UnmarshalPrimitive(m, "name", &obj.Name)
	if err != nil {
		err = core.SDKErrorf(err, "", "name-error", common.GetComponentInfo())
		return
	}
	err = core.UnmarshalPrimitive(m, "id", &obj.ID)
	if err != nil {
		err = core.SDKErrorf(err, "", "id-error", common.GetComponentInfo())
		return
	}
	err = core.UnmarshalPrimitive(m, "created_at", &obj.CreatedAt)
	if err != nil {
		err = core.SDKErrorf(err, "", "created_at-error", common.GetComponentInfo())
		return
	}
	err = core.UnmarshalPrimitive(m, "created_by", &obj.CreatedBy)
	if err != nil {
		err = core.SDKErrorf(err, "", "created_by-error", common.GetComponentInfo())
		return
	}
	reflect.ValueOf(result).Elem().Set(reflect.ValueOf(obj))
	return
}

// KMIPObject : Properties applicable to all KMIP object resources.
type KMIPObject struct {
	// The v4 UUID that uniquely identifies this KMIP object.
	ID *string `json:"id" validate:"required"`

	// The object type of the kmip object according to the KMIP specification. Currently, only kmip_object_type 2(Symmetric
	// Key) is supported. For more info on the KMIP specification and object types, read
	// https://docs.oasis-open.org/kmip/spec/v1.4/os/kmip-spec-v1.4-os.html#_Toc490660932.
	KmipObjectType *int64 `json:"kmip_object_type" validate:"required"`

	// States are integers and correspond to Pre-Active = 1, Active = 2, Deactivated = 3, Compromised = 4, Destroyed = 5,
	// Destroyed Compromised = 6. For more info on the KMIP specification, read
	// https://docs.oasis-open.org/kmip/spec/v1.4/os/kmip-spec-v1.4-os.html.
	State *int64 `json:"state,omitempty"`

	// The date the KMIP object was created. The date format follows RFC 3339.
	CreatedAt *strfmt.DateTime `json:"created_at" validate:"required"`

	// The v4 UUID that uniquely identifies the certificate used to create this KMIP object.
	CreatedByKmipClientCertID *string `json:"created_by_kmip_client_cert_id" validate:"required"`

	// The IAM id that created the certificate resource used to create this KMIP object.
	CreatedBy *string `json:"created_by,omitempty"`

	// The date the KMIP object was last modified. The date format follows RFC 3339.
	UpdatedAt *strfmt.DateTime `json:"updated_at,omitempty"`

	// The v4 UUID that uniquely identifies the certificate used to update this KMIP object.
	UpdatedByKmipClientCertID *string `json:"updated_by_kmip_client_cert_id,omitempty"`

	// The IAM id that created the certificate resource used to update this KMIP object.
	UpdatedBy *string `json:"updated_by,omitempty"`

	// The date the KMIP object was destroyed. The date format follows RFC 3339.
	DestroyedAt *strfmt.DateTime `json:"destroyed_at,omitempty"`

	// The v4 UUID that uniquely identifies the certificate used to destroy this KMIP object.
	DestroyedByKmipClientCertID *string `json:"destroyed_by_kmip_client_cert_id,omitempty"`

	// The IAM id that created the certificate resource used to destroy this KMIP object.
	DestroyedBy *string `json:"destroyed_by,omitempty"`

	// A boolean that specifies if the object has the ability to be restored.
	Recoverable *bool `json:"recoverable,omitempty"`
}

// UnmarshalKMIPObject unmarshals an instance of KMIPObject from the specified map of raw messages.
func UnmarshalKMIPObject(m map[string]json.RawMessage, result interface{}) (err error) {
	obj := new(KMIPObject)
	err = core.UnmarshalPrimitive(m, "id", &obj.ID)
	if err != nil {
		err = core.SDKErrorf(err, "", "id-error", common.GetComponentInfo())
		return
	}
	err = core.UnmarshalPrimitive(m, "kmip_object_type", &obj.KmipObjectType)
	if err != nil {
		err = core.SDKErrorf(err, "", "kmip_object_type-error", common.GetComponentInfo())
		return
	}
	err = core.UnmarshalPrimitive(m, "state", &obj.State)
	if err != nil {
		err = core.SDKErrorf(err, "", "state-error", common.GetComponentInfo())
		return
	}
	err = core.UnmarshalPrimitive(m, "created_at", &obj.CreatedAt)
	if err != nil {
		err = core.SDKErrorf(err, "", "created_at-error", common.GetComponentInfo())
		return
	}
	err = core.UnmarshalPrimitive(m, "created_by_kmip_client_cert_id", &obj.CreatedByKmipClientCertID)
	if err != nil {
		err = core.SDKErrorf(err, "", "created_by_kmip_client_cert_id-error", common.GetComponentInfo())
		return
	}
	err = core.UnmarshalPrimitive(m, "created_by", &obj.CreatedBy)
	if err != nil {
		err = core.SDKErrorf(err, "", "created_by-error", common.GetComponentInfo())
		return
	}
	err = core.UnmarshalPrimitive(m, "updated_at", &obj.UpdatedAt)
	if err != nil {
		err = core.SDKErrorf(err, "", "updated_at-error", common.GetComponentInfo())
		return
	}
	err = core.UnmarshalPrimitive(m, "updated_by_kmip_client_cert_id", &obj.UpdatedByKmipClientCertID)
	if err != nil {
		err = core.SDKErrorf(err, "", "updated_by_kmip_client_cert_id-error", common.GetComponentInfo())
		return
	}
	err = core.UnmarshalPrimitive(m, "updated_by", &obj.UpdatedBy)
	if err != nil {
		err = core.SDKErrorf(err, "", "updated_by-error", common.GetComponentInfo())
		return
	}
	err = core.UnmarshalPrimitive(m, "destroyed_at", &obj.DestroyedAt)
	if err != nil {
		err = core.SDKErrorf(err, "", "destroyed_at-error", common.GetComponentInfo())
		return
	}
	err = core.UnmarshalPrimitive(m, "destroyed_by_kmip_client_cert_id", &obj.DestroyedByKmipClientCertID)
	if err != nil {
		err = core.SDKErrorf(err, "", "destroyed_by_kmip_client_cert_id-error", common.GetComponentInfo())
		return
	}
	err = core.UnmarshalPrimitive(m, "destroyed_by", &obj.DestroyedBy)
	if err != nil {
		err = core.SDKErrorf(err, "", "destroyed_by-error", common.GetComponentInfo())
		return
	}
	err = core.UnmarshalPrimitive(m, "recoverable", &obj.Recoverable)
	if err != nil {
		err = core.SDKErrorf(err, "", "recoverable-error", common.GetComponentInfo())
		return
	}
	reflect.ValueOf(result).Elem().Set(reflect.ValueOf(obj))
	return
}

// KMIPProfileDataBody : The data specific to the KMIP Adapter profile. This is a required field for profile `native_1.0`.
// Models which "extend" this model:
// - KMIPProfileDataBodyKMIPProfileDataNative
type KMIPProfileDataBody struct {
	// An ID that identifies the Customer Root Key(CRK) to be used. This CRK must exist in the same kms instance as the
	// adapter.
	CrkID *string `json:"crk_id,omitempty"`
}

func (*KMIPProfileDataBody) isaKMIPProfileDataBody() bool {
	return true
}

type KMIPProfileDataBodyIntf interface {
	isaKMIPProfileDataBody() bool
}

// UnmarshalKMIPProfileDataBody unmarshals an instance of KMIPProfileDataBody from the specified map of raw messages.
func UnmarshalKMIPProfileDataBody(m map[string]json.RawMessage, result interface{}) (err error) {
	obj := new(KMIPProfileDataBody)
	err = core.UnmarshalPrimitive(m, "crk_id", &obj.CrkID)
	if err != nil {
		err = core.SDKErrorf(err, "", "crk_id-error", common.GetComponentInfo())
		return
	}
	reflect.ValueOf(result).Elem().Set(reflect.ValueOf(obj))
	return
}

// Key : Properties associated with a key response.
type Key struct {
	Metadata CollectionMetadataOneOfIntf `json:"metadata,omitempty"`

	// A collection of resources.
	Resources []KeyWithPayload `json:"resources,omitempty"`
}

// UnmarshalKey unmarshals an instance of Key from the specified map of raw messages.
func UnmarshalKey(m map[string]json.RawMessage, result interface{}) (err error) {
	obj := new(Key)
	err = core.UnmarshalModel(m, "metadata", &obj.Metadata, UnmarshalCollectionMetadataOneOf)
	if err != nil {
		err = core.SDKErrorf(err, "", "metadata-error", common.GetComponentInfo())
		return
	}
	err = core.UnmarshalModel(m, "resources", &obj.Resources, UnmarshalKeyWithPayload)
	if err != nil {
		err = core.SDKErrorf(err, "", "resources-error", common.GetComponentInfo())
		return
	}
	reflect.ValueOf(result).Elem().Set(reflect.ValueOf(obj))
	return
}

// KeyActionOneOfResponse : KeyActionOneOfResponse struct
// Models which "extend" this model:
// - KeyActionOneOfResponseWrapKeyResponseBody
// - KeyActionOneOfResponseUnwrapKeyResponseBody
// - KeyActionOneOfResponseRewrapKeyResponseBody
type KeyActionOneOfResponse struct {
	// The data encryption key (DEK) used in wrap actions when the query parameter is set to `wrap`. The system returns a
	// base64 encoded plaintext in the response entity-body when you perform an `unwrap` action on a key. To wrap an
	// existing DEK, provide a base64 encoded plaintext during a `wrap` action. To generate a new DEK, omit the `plaintext`
	// property. Key Protect generates a random plaintext (32 bytes) that is rooted in an HSM and then wraps that value.
	// **Note:** When you unwrap a wrapped data encryption key (WDEK) by using a rotated root key, the service returns a
	// new ciphertext in the response entity-body. Each ciphertext remains available for `unwrap` actions. If you unwrap a
	// DEK with a previous ciphertext, the service also returns the latest ciphertext in the response. Use the latest
	// ciphertext for future unwrap operations.
	Plaintext *string `json:"plaintext,omitempty"`

	// The wrapped data encryption key (WDEK) that you can export to your app or service. The ciphertext contains the DEK
	// wrapped by the latest version of the key (WDEK). It is recommended to store and use this WDEK in future calls to Key
	// Protect. The value is base64 encoded.
	Ciphertext *string `json:"ciphertext,omitempty"`

	// The key version that was used to wrap the DEK. This key version is associated with the `ciphertext` value that was
	// used in the request.
	KeyVersion *WrappedKeyVersionKeyVersion `json:"keyVersion,omitempty"`

	// The latest key version that was used to rewrap the DEK. This key version is associated with the `ciphertext` value
	// that's returned in the response.
	RewrappedKeyVersion *RewrappedKeyVersionRewrappedKeyVersion `json:"rewrappedKeyVersion,omitempty"`
}

func (*KeyActionOneOfResponse) isaKeyActionOneOfResponse() bool {
	return true
}

type KeyActionOneOfResponseIntf interface {
	isaKeyActionOneOfResponse() bool
}

// UnmarshalKeyActionOneOfResponse unmarshals an instance of KeyActionOneOfResponse from the specified map of raw messages.
func UnmarshalKeyActionOneOfResponse(m map[string]json.RawMessage, result interface{}) (err error) {
	obj := new(KeyActionOneOfResponse)
	err = core.UnmarshalPrimitive(m, "plaintext", &obj.Plaintext)
	if err != nil {
		err = core.SDKErrorf(err, "", "plaintext-error", common.GetComponentInfo())
		return
	}
	err = core.UnmarshalPrimitive(m, "ciphertext", &obj.Ciphertext)
	if err != nil {
		err = core.SDKErrorf(err, "", "ciphertext-error", common.GetComponentInfo())
		return
	}
	err = core.UnmarshalModel(m, "keyVersion", &obj.KeyVersion, UnmarshalWrappedKeyVersionKeyVersion)
	if err != nil {
		err = core.SDKErrorf(err, "", "keyVersion-error", common.GetComponentInfo())
		return
	}
	err = core.UnmarshalModel(m, "rewrappedKeyVersion", &obj.RewrappedKeyVersion, UnmarshalRewrappedKeyVersionRewrappedKeyVersion)
	if err != nil {
		err = core.SDKErrorf(err, "", "rewrappedKeyVersion-error", common.GetComponentInfo())
		return
	}
	reflect.ValueOf(result).Elem().Set(reflect.ValueOf(obj))
	return
}

// KeyAlias : Properties associated with a specific key alias.
type KeyAlias struct {
	// The metadata that describes the resource array.
	Metadata *CollectionMetadata `json:"metadata,omitempty"`

	// A collection of resources.
	Resources []KeyAliasResource `json:"resources,omitempty"`
}

// UnmarshalKeyAlias unmarshals an instance of KeyAlias from the specified map of raw messages.
func UnmarshalKeyAlias(m map[string]json.RawMessage, result interface{}) (err error) {
	obj := new(KeyAlias)
	err = core.UnmarshalModel(m, "metadata", &obj.Metadata, UnmarshalCollectionMetadata)
	if err != nil {
		err = core.SDKErrorf(err, "", "metadata-error", common.GetComponentInfo())
		return
	}
	err = core.UnmarshalModel(m, "resources", &obj.Resources, UnmarshalKeyAliasResource)
	if err != nil {
		err = core.SDKErrorf(err, "", "resources-error", common.GetComponentInfo())
		return
	}
	reflect.ValueOf(result).Elem().Set(reflect.ValueOf(obj))
	return
}

// KeyAliasResource : Properties associated with an alias.
type KeyAliasResource struct {
	// The ID that identifies the key that is associated with the alias.
	KeyID *string `json:"keyId,omitempty"`

	// The unique, human-readable alias assigned to the key.
	Alias *string `json:"alias,omitempty"`

	// The unique identifier for the user that created the alias.
	CreatedBy *string `json:"createdBy,omitempty"`

	// The date the alias was created. The date format follows RFC 3339.
	CreationDate *strfmt.DateTime `json:"creationDate,omitempty"`
}

// UnmarshalKeyAliasResource unmarshals an instance of KeyAliasResource from the specified map of raw messages.
func UnmarshalKeyAliasResource(m map[string]json.RawMessage, result interface{}) (err error) {
	obj := new(KeyAliasResource)
	err = core.UnmarshalPrimitive(m, "keyId", &obj.KeyID)
	if err != nil {
		err = core.SDKErrorf(err, "", "keyId-error", common.GetComponentInfo())
		return
	}
	err = core.UnmarshalPrimitive(m, "alias", &obj.Alias)
	if err != nil {
		err = core.SDKErrorf(err, "", "alias-error", common.GetComponentInfo())
		return
	}
	err = core.UnmarshalPrimitive(m, "createdBy", &obj.CreatedBy)
	if err != nil {
		err = core.SDKErrorf(err, "", "createdBy-error", common.GetComponentInfo())
		return
	}
	err = core.UnmarshalPrimitive(m, "creationDate", &obj.CreationDate)
	if err != nil {
		err = core.SDKErrorf(err, "", "creationDate-error", common.GetComponentInfo())
		return
	}
	reflect.ValueOf(result).Elem().Set(reflect.ValueOf(obj))
	return
}

// KeyFullRepresentation : Properties returned only for DELETE.
type KeyFullRepresentation struct {
	// Specifies the MIME type that represents the key resource. Currently, only the default is supported.
	Type *string `json:"type,omitempty"`

	// The v4 UUID used to uniquely identify the resource, as specified by RFC 4122.
	ID *string `json:"id,omitempty"`

	// A human-readable name assigned to your key for convenience. To protect your privacy do not use personal data, such
	// as your name or location, as the name for your key.
	Name *string `json:"name,omitempty"`

	// One or more, up to a total of five, human-readable unique aliases assigned to your key. To protect your privacy do
	// not use personal data, such as your name or location, as an alias for your key. Each alias must be alphanumeric and
	// cannot contain spaces or special characters other than `-` or `_`. The alias cannot be a UUID and must not be a Key
	// Protect reserved name: `allowed_ip`, `key`, `keys`, `metadata`, `policy`, `policies`, `registration`,
	// `registrations`, `ring`, `rings`, `rotate`, `wrap`, `unwrap`, `rewrap`, `version`, `versions`.
	Aliases []string `json:"aliases,omitempty"`

	// A text field used to provide a more detailed description of the key. The maximum length is 240 characters. To
	// protect your privacy, do not use personal data, such as your name or location, as a description for your key.
	Description *string `json:"description,omitempty"`

	// Up to 30 tags can be created. Tags can be between 0-30 characters, including spaces. Special characters not
	// permitted include angled brackets, comma, colon, ampersand, and vertical pipe character (|). To protect your
	// privacy, do not use personal data, such as your name or location, as a tag for your key.
	Tags []string `json:"tags,omitempty"`

	// The key state based on NIST SP 800-57. States are integers and correspond to the Pre-activation = 0, Active = 1,
	// Suspended = 2, Deactivated = 3, and Destroyed = 5 values.
	State *int64 `json:"state,omitempty"`

	// The date and time that the key expires in the system, in RFC 3339 format (YYYY-MM-DD HH:MM:SS.SS, for example
	// 2019-10-12T07:20:50.52Z). Keys created with an expiration date automatically transition to the Deactivated state
	// within one hour after expiration. In this state, the only allowed actions on the key are unwrap, rewrap, rotate, and
	// delete. Deactivated keys cannot be used to encrypt (wrap) new data, even if rotated while deactivated. Rotation does
	// not reset or extend the expiration date, nor does it allow the date to be changed. It is recommended that any data
	// encrypted with an expiring or expired key be re-encrypted using a new customer root key (CRK) before the original
	// CRK expires, to prevent service disruptions. Deleting and restoring a deactivated key does not move it back to the
	// Active state. If the expirationDate attribute is omitted, the key does not expire.
	ExpirationDate *strfmt.DateTime `json:"expirationDate,omitempty"`

	// A boolean that determines whether the key material can leave the service. If set to `false`, Key Protect designates
	// the key as a nonextractable root key used for `wrap` and `unwrap` actions. If set to `true`, Key Protect designates
	// the key as a standard key that you can store in your apps and services. Once set to `false` it cannot be changed to
	// `true`.
	Extractable *bool `json:"extractable,omitempty"`

	// The Cloud Resource Name (CRN) that uniquely identifies your cloud resources.
	Crn *string `json:"crn,omitempty"`

	// A boolean that shows whether your key was originally imported or generated in Key Protect. The value is set by Key
	// Protect based on how the key material is initially added to the service. A value of `true` indicates that you must
	// provide new key material when it's time to rotate the key. A value of `false` indicates that Key Protect will
	// generate the new key material on a `rotate` operation, as it did in key creation.
	Imported *bool `json:"imported,omitempty"`

	// An ID that identifies the key ring. Each ID is unique only within the given instance and is not reserved across the
	// Key Protect service.
	KeyRingID *string `json:"keyRingID,omitempty"`

	// The date the key material was created. The date format follows RFC 3339.
	CreationDate *strfmt.DateTime `json:"creationDate,omitempty"`

	// The unique identifier for the resource that created the key.
	CreatedBy *string `json:"createdBy,omitempty"`

	// Deprecated.
	// Deprecated: this field is deprecated and may be removed in a future release.
	AlgorithmType *string `json:"algorithmType,omitempty"`

	// Deprecated.
	AlgorithmMetadata *KeyFullRepresentationAlgorithmMetadata `json:"algorithmMetadata,omitempty"`

	// Deprecated.
	// Deprecated: this field is deprecated and may be removed in a future release.
	AlgorithmBitSize *int64 `json:"algorithmBitSize,omitempty"`

	// Deprecated.
	// Deprecated: this field is deprecated and may be removed in a future release.
	AlgorithmMode *string `json:"algorithmMode,omitempty"`

	// A code indicating the reason the key is not in the activation state.
	NonactiveStateReason *int64 `json:"nonactiveStateReason,omitempty"`

	// Updates when any part of the key metadata is modified. The date format follows RFC 3339.
	LastUpdateDate *strfmt.DateTime `json:"lastUpdateDate,omitempty"`

	// Updates to show when the key was last rotated. The date format follows RFC 3339.
	LastRotateDate *strfmt.DateTime `json:"lastRotateDate,omitempty"`

	// Properties associated with a specific key version.
	KeyVersion *KeyVersion `json:"keyVersion,omitempty"`

	// Metadata that indicates the status of a dual authorization policy on the key.
	DualAuthDelete *DualAuthKeyMetadata `json:"dualAuthDelete,omitempty"`

	// Metadata that indicates the status of a rotation policy on the key.
	Rotation *RotationKeyMetadata `json:"rotation,omitempty"`

	// A boolean that determines whether the key has been deleted.
	Deleted *bool `json:"deleted,omitempty"`

	// The date the key material was destroyed. The date format follows RFC 3339.
	DeletionDate *strfmt.DateTime `json:"deletionDate,omitempty"`

	// The unique identifier for the resource that deleted the key.
	DeletedBy *string `json:"deletedBy,omitempty"`

	// The date the key will no longer have the ability to be restored.
	RestoreExpirationDate *strfmt.DateTime `json:"restoreExpirationDate,omitempty"`

	// A boolean that specifies if your key has the ability to be restored. A value of `true` indicates that the key can be
	// restored. A value of `false` indicates that the key is unable to be restored.
	RestoreAllowed *bool `json:"restoreAllowed,omitempty"`

	// A boolean that specifies if the key can be purged. A value of `true` indicates that the key can be purged. A value
	// of `false` indicates that the key is within the purge wait period and is not ready to be purged.
	PurgeAllowed *bool `json:"purgeAllowed,omitempty"`

	// The date the key will be ready to be purged.
	PurgeAllowedFrom *strfmt.DateTime `json:"purgeAllowedFrom,omitempty"`

	// The date the deleted key will be automatically purged from Key Protect system.
	PurgeScheduledOn *strfmt.DateTime `json:"purgeScheduledOn,omitempty"`
}

// Constants associated with the KeyFullRepresentation.Type property.
// Specifies the MIME type that represents the key resource. Currently, only the default is supported.
const (
	KeyFullRepresentation_Type_ApplicationVndIbmKmsKeyJSON = "application/vnd.ibm.kms.key+json"
)

// Constants associated with the KeyFullRepresentation.AlgorithmType property.
// Deprecated.
const (
	KeyFullRepresentation_AlgorithmType_Aes        = "AES"
	KeyFullRepresentation_AlgorithmType_Deprecated = "Deprecated"
)

// Constants associated with the KeyFullRepresentation.AlgorithmMode property.
// Deprecated.
const (
	KeyFullRepresentation_AlgorithmMode_CbcPad     = "CBC_PAD"
	KeyFullRepresentation_AlgorithmMode_Deprecated = "Deprecated"
)

// UnmarshalKeyFullRepresentation unmarshals an instance of KeyFullRepresentation from the specified map of raw messages.
func UnmarshalKeyFullRepresentation(m map[string]json.RawMessage, result interface{}) (err error) {
	obj := new(KeyFullRepresentation)
	err = core.UnmarshalPrimitive(m, "type", &obj.Type)
	if err != nil {
		err = core.SDKErrorf(err, "", "type-error", common.GetComponentInfo())
		return
	}
	err = core.UnmarshalPrimitive(m, "id", &obj.ID)
	if err != nil {
		err = core.SDKErrorf(err, "", "id-error", common.GetComponentInfo())
		return
	}
	err = core.UnmarshalPrimitive(m, "name", &obj.Name)
	if err != nil {
		err = core.SDKErrorf(err, "", "name-error", common.GetComponentInfo())
		return
	}
	err = core.UnmarshalPrimitive(m, "aliases", &obj.Aliases)
	if err != nil {
		err = core.SDKErrorf(err, "", "aliases-error", common.GetComponentInfo())
		return
	}
	err = core.UnmarshalPrimitive(m, "description", &obj.Description)
	if err != nil {
		err = core.SDKErrorf(err, "", "description-error", common.GetComponentInfo())
		return
	}
	err = core.UnmarshalPrimitive(m, "tags", &obj.Tags)
	if err != nil {
		err = core.SDKErrorf(err, "", "tags-error", common.GetComponentInfo())
		return
	}
	err = core.UnmarshalPrimitive(m, "state", &obj.State)
	if err != nil {
		err = core.SDKErrorf(err, "", "state-error", common.GetComponentInfo())
		return
	}
	err = core.UnmarshalPrimitive(m, "expirationDate", &obj.ExpirationDate)
	if err != nil {
		err = core.SDKErrorf(err, "", "expirationDate-error", common.GetComponentInfo())
		return
	}
	err = core.UnmarshalPrimitive(m, "extractable", &obj.Extractable)
	if err != nil {
		err = core.SDKErrorf(err, "", "extractable-error", common.GetComponentInfo())
		return
	}
	err = core.UnmarshalPrimitive(m, "crn", &obj.Crn)
	if err != nil {
		err = core.SDKErrorf(err, "", "crn-error", common.GetComponentInfo())
		return
	}
	err = core.UnmarshalPrimitive(m, "imported", &obj.Imported)
	if err != nil {
		err = core.SDKErrorf(err, "", "imported-error", common.GetComponentInfo())
		return
	}
	err = core.UnmarshalPrimitive(m, "keyRingID", &obj.KeyRingID)
	if err != nil {
		err = core.SDKErrorf(err, "", "keyRingID-error", common.GetComponentInfo())
		return
	}
	err = core.UnmarshalPrimitive(m, "creationDate", &obj.CreationDate)
	if err != nil {
		err = core.SDKErrorf(err, "", "creationDate-error", common.GetComponentInfo())
		return
	}
	err = core.UnmarshalPrimitive(m, "createdBy", &obj.CreatedBy)
	if err != nil {
		err = core.SDKErrorf(err, "", "createdBy-error", common.GetComponentInfo())
		return
	}
	err = core.UnmarshalPrimitive(m, "algorithmType", &obj.AlgorithmType)
	if err != nil {
		err = core.SDKErrorf(err, "", "algorithmType-error", common.GetComponentInfo())
		return
	}
	err = core.UnmarshalModel(m, "algorithmMetadata", &obj.AlgorithmMetadata, UnmarshalKeyFullRepresentationAlgorithmMetadata)
	if err != nil {
		err = core.SDKErrorf(err, "", "algorithmMetadata-error", common.GetComponentInfo())
		return
	}
	err = core.UnmarshalPrimitive(m, "algorithmBitSize", &obj.AlgorithmBitSize)
	if err != nil {
		err = core.SDKErrorf(err, "", "algorithmBitSize-error", common.GetComponentInfo())
		return
	}
	err = core.UnmarshalPrimitive(m, "algorithmMode", &obj.AlgorithmMode)
	if err != nil {
		err = core.SDKErrorf(err, "", "algorithmMode-error", common.GetComponentInfo())
		return
	}
	err = core.UnmarshalPrimitive(m, "nonactiveStateReason", &obj.NonactiveStateReason)
	if err != nil {
		err = core.SDKErrorf(err, "", "nonactiveStateReason-error", common.GetComponentInfo())
		return
	}
	err = core.UnmarshalPrimitive(m, "lastUpdateDate", &obj.LastUpdateDate)
	if err != nil {
		err = core.SDKErrorf(err, "", "lastUpdateDate-error", common.GetComponentInfo())
		return
	}
	err = core.UnmarshalPrimitive(m, "lastRotateDate", &obj.LastRotateDate)
	if err != nil {
		err = core.SDKErrorf(err, "", "lastRotateDate-error", common.GetComponentInfo())
		return
	}
	err = core.UnmarshalModel(m, "keyVersion", &obj.KeyVersion, UnmarshalKeyVersion)
	if err != nil {
		err = core.SDKErrorf(err, "", "keyVersion-error", common.GetComponentInfo())
		return
	}
	err = core.UnmarshalModel(m, "dualAuthDelete", &obj.DualAuthDelete, UnmarshalDualAuthKeyMetadata)
	if err != nil {
		err = core.SDKErrorf(err, "", "dualAuthDelete-error", common.GetComponentInfo())
		return
	}
	err = core.UnmarshalModel(m, "rotation", &obj.Rotation, UnmarshalRotationKeyMetadata)
	if err != nil {
		err = core.SDKErrorf(err, "", "rotation-error", common.GetComponentInfo())
		return
	}
	err = core.UnmarshalPrimitive(m, "deleted", &obj.Deleted)
	if err != nil {
		err = core.SDKErrorf(err, "", "deleted-error", common.GetComponentInfo())
		return
	}
	err = core.UnmarshalPrimitive(m, "deletionDate", &obj.DeletionDate)
	if err != nil {
		err = core.SDKErrorf(err, "", "deletionDate-error", common.GetComponentInfo())
		return
	}
	err = core.UnmarshalPrimitive(m, "deletedBy", &obj.DeletedBy)
	if err != nil {
		err = core.SDKErrorf(err, "", "deletedBy-error", common.GetComponentInfo())
		return
	}
	err = core.UnmarshalPrimitive(m, "restoreExpirationDate", &obj.RestoreExpirationDate)
	if err != nil {
		err = core.SDKErrorf(err, "", "restoreExpirationDate-error", common.GetComponentInfo())
		return
	}
	err = core.UnmarshalPrimitive(m, "restoreAllowed", &obj.RestoreAllowed)
	if err != nil {
		err = core.SDKErrorf(err, "", "restoreAllowed-error", common.GetComponentInfo())
		return
	}
	err = core.UnmarshalPrimitive(m, "purgeAllowed", &obj.PurgeAllowed)
	if err != nil {
		err = core.SDKErrorf(err, "", "purgeAllowed-error", common.GetComponentInfo())
		return
	}
	err = core.UnmarshalPrimitive(m, "purgeAllowedFrom", &obj.PurgeAllowedFrom)
	if err != nil {
		err = core.SDKErrorf(err, "", "purgeAllowedFrom-error", common.GetComponentInfo())
		return
	}
	err = core.UnmarshalPrimitive(m, "purgeScheduledOn", &obj.PurgeScheduledOn)
	if err != nil {
		err = core.SDKErrorf(err, "", "purgeScheduledOn-error", common.GetComponentInfo())
		return
	}
	reflect.ValueOf(result).Elem().Set(reflect.ValueOf(obj))
	return
}

// KeyFullRepresentationAlgorithmMetadata : Deprecated.
type KeyFullRepresentationAlgorithmMetadata struct {
	// Deprecated.
	BitLength *string `json:"bitLength,omitempty"`

	// Deprecated.
	Mode *string `json:"mode,omitempty"`
}

// Constants associated with the KeyFullRepresentationAlgorithmMetadata.Mode property.
// Deprecated.
const (
	KeyFullRepresentationAlgorithmMetadata_Mode_CbcPad     = "CBC_PAD"
	KeyFullRepresentationAlgorithmMetadata_Mode_Deprecated = "Deprecated"
)

// UnmarshalKeyFullRepresentationAlgorithmMetadata unmarshals an instance of KeyFullRepresentationAlgorithmMetadata from the specified map of raw messages.
func UnmarshalKeyFullRepresentationAlgorithmMetadata(m map[string]json.RawMessage, result interface{}) (err error) {
	obj := new(KeyFullRepresentationAlgorithmMetadata)
	err = core.UnmarshalPrimitive(m, "bitLength", &obj.BitLength)
	if err != nil {
		err = core.SDKErrorf(err, "", "bitLength-error", common.GetComponentInfo())
		return
	}
	err = core.UnmarshalPrimitive(m, "mode", &obj.Mode)
	if err != nil {
		err = core.SDKErrorf(err, "", "mode-error", common.GetComponentInfo())
		return
	}
	reflect.ValueOf(result).Elem().Set(reflect.ValueOf(obj))
	return
}

// KeyPolicyDualAuthDelete : Properties that are associated with key level dual authorization delete policy.
type KeyPolicyDualAuthDelete struct {
	// Specifies the MIME type that represents the policy resource. Currently, only the default is supported.
	Type *string `json:"type" validate:"required"`

	// Data associated with the dual authorization delete policy.
	DualAuthDelete *KeyPolicyDualAuthDeleteDualAuthDelete `json:"dualAuthDelete" validate:"required"`
}

// Constants associated with the KeyPolicyDualAuthDelete.Type property.
// Specifies the MIME type that represents the policy resource. Currently, only the default is supported.
const (
	KeyPolicyDualAuthDelete_Type_ApplicationVndIbmKmsPolicyJSON = "application/vnd.ibm.kms.policy+json"
)

// NewKeyPolicyDualAuthDelete : Instantiate KeyPolicyDualAuthDelete (Generic Model Constructor)
func (*IbmKeyProtectApiV2) NewKeyPolicyDualAuthDelete(typeVar string, dualAuthDelete *KeyPolicyDualAuthDeleteDualAuthDelete) (_model *KeyPolicyDualAuthDelete, err error) {
	_model = &KeyPolicyDualAuthDelete{
		Type:           core.StringPtr(typeVar),
		DualAuthDelete: dualAuthDelete,
	}
	err = core.ValidateStruct(_model, "required parameters")
	if err != nil {
		err = core.SDKErrorf(err, "", "model-missing-required", common.GetComponentInfo())
	}
	return
}

// UnmarshalKeyPolicyDualAuthDelete unmarshals an instance of KeyPolicyDualAuthDelete from the specified map of raw messages.
func UnmarshalKeyPolicyDualAuthDelete(m map[string]json.RawMessage, result interface{}) (err error) {
	obj := new(KeyPolicyDualAuthDelete)
	err = core.UnmarshalPrimitive(m, "type", &obj.Type)
	if err != nil {
		err = core.SDKErrorf(err, "", "type-error", common.GetComponentInfo())
		return
	}
	err = core.UnmarshalModel(m, "dualAuthDelete", &obj.DualAuthDelete, UnmarshalKeyPolicyDualAuthDeleteDualAuthDelete)
	if err != nil {
		err = core.SDKErrorf(err, "", "dualAuthDelete-error", common.GetComponentInfo())
		return
	}
	reflect.ValueOf(result).Elem().Set(reflect.ValueOf(obj))
	return
}

// KeyPolicyDualAuthDeleteDualAuthDelete : Data associated with the dual authorization delete policy.
type KeyPolicyDualAuthDeleteDualAuthDelete struct {
	// If set to `true`, Key Protect enables a dual authorization policy on a single key. After you enable the policy, Key
	// Protect requires an authorization from two users to delete this key. For example, you can authorize the deletion
	// first by using the [SetKeyForDeletion](#invoke-an-action-on-a-key) action. Then, a different user provides a second
	// authorization implicitly by calling `DELETE /keys` to delete the key.
	// **Note:** Once the dual authorization policy is set on the key, it cannot be reverted.
	Enabled *bool `json:"enabled" validate:"required"`
}

// NewKeyPolicyDualAuthDeleteDualAuthDelete : Instantiate KeyPolicyDualAuthDeleteDualAuthDelete (Generic Model Constructor)
func (*IbmKeyProtectApiV2) NewKeyPolicyDualAuthDeleteDualAuthDelete(enabled bool) (_model *KeyPolicyDualAuthDeleteDualAuthDelete, err error) {
	_model = &KeyPolicyDualAuthDeleteDualAuthDelete{
		Enabled: core.BoolPtr(enabled),
	}
	err = core.ValidateStruct(_model, "required parameters")
	if err != nil {
		err = core.SDKErrorf(err, "", "model-missing-required", common.GetComponentInfo())
	}
	return
}

// UnmarshalKeyPolicyDualAuthDeleteDualAuthDelete unmarshals an instance of KeyPolicyDualAuthDeleteDualAuthDelete from the specified map of raw messages.
func UnmarshalKeyPolicyDualAuthDeleteDualAuthDelete(m map[string]json.RawMessage, result interface{}) (err error) {
	obj := new(KeyPolicyDualAuthDeleteDualAuthDelete)
	err = core.UnmarshalPrimitive(m, "enabled", &obj.Enabled)
	if err != nil {
		err = core.SDKErrorf(err, "", "enabled-error", common.GetComponentInfo())
		return
	}
	reflect.ValueOf(result).Elem().Set(reflect.ValueOf(obj))
	return
}

// KeyPolicyRotation : KeyPolicyRotation struct
type KeyPolicyRotation struct {
	// Specifies the MIME type that represents the policy resource. Currently, only the default is supported.
	Type *string `json:"type" validate:"required"`

	// Data associated with the automatic key rotation policy.
	Rotation *KeyPolicyRotationRotation `json:"rotation" validate:"required"`
}

// Constants associated with the KeyPolicyRotation.Type property.
// Specifies the MIME type that represents the policy resource. Currently, only the default is supported.
const (
	KeyPolicyRotation_Type_ApplicationVndIbmKmsPolicyJSON = "application/vnd.ibm.kms.policy+json"
)

// NewKeyPolicyRotation : Instantiate KeyPolicyRotation (Generic Model Constructor)
func (*IbmKeyProtectApiV2) NewKeyPolicyRotation(typeVar string, rotation *KeyPolicyRotationRotation) (_model *KeyPolicyRotation, err error) {
	_model = &KeyPolicyRotation{
		Type:     core.StringPtr(typeVar),
		Rotation: rotation,
	}
	err = core.ValidateStruct(_model, "required parameters")
	if err != nil {
		err = core.SDKErrorf(err, "", "model-missing-required", common.GetComponentInfo())
	}
	return
}

// UnmarshalKeyPolicyRotation unmarshals an instance of KeyPolicyRotation from the specified map of raw messages.
func UnmarshalKeyPolicyRotation(m map[string]json.RawMessage, result interface{}) (err error) {
	obj := new(KeyPolicyRotation)
	err = core.UnmarshalPrimitive(m, "type", &obj.Type)
	if err != nil {
		err = core.SDKErrorf(err, "", "type-error", common.GetComponentInfo())
		return
	}
	err = core.UnmarshalModel(m, "rotation", &obj.Rotation, UnmarshalKeyPolicyRotationRotation)
	if err != nil {
		err = core.SDKErrorf(err, "", "rotation-error", common.GetComponentInfo())
		return
	}
	reflect.ValueOf(result).Elem().Set(reflect.ValueOf(obj))
	return
}

// KeyPolicyRotationNonRequiredRotation : Data associated with the automatic key rotation policy.
type KeyPolicyRotationNonRequiredRotation struct {
	// If set to `true`, Key Protect enables a rotation policy on a single key.
	Enabled *bool `json:"enabled" validate:"required"`

	// Specifies the key rotation time interval in approximate months standardized to 30 days each.  A minimum of 1 and a
	// maximum of 12 can be set.
	IntervalMonth *int64 `json:"interval_month" validate:"required"`
}

// UnmarshalKeyPolicyRotationNonRequiredRotation unmarshals an instance of KeyPolicyRotationNonRequiredRotation from the specified map of raw messages.
func UnmarshalKeyPolicyRotationNonRequiredRotation(m map[string]json.RawMessage, result interface{}) (err error) {
	obj := new(KeyPolicyRotationNonRequiredRotation)
	err = core.UnmarshalPrimitive(m, "enabled", &obj.Enabled)
	if err != nil {
		err = core.SDKErrorf(err, "", "enabled-error", common.GetComponentInfo())
		return
	}
	err = core.UnmarshalPrimitive(m, "interval_month", &obj.IntervalMonth)
	if err != nil {
		err = core.SDKErrorf(err, "", "interval_month-error", common.GetComponentInfo())
		return
	}
	reflect.ValueOf(result).Elem().Set(reflect.ValueOf(obj))
	return
}

// KeyPolicyRotationRotation : Data associated with the automatic key rotation policy.
type KeyPolicyRotationRotation struct {
	// If set to `true`, Key Protect enables a rotation policy on a single key.
	Enabled *bool `json:"enabled" validate:"required"`

	// Specifies the key rotation time interval in approximate months standardized to 30 days each. A minimum of 1 and a
	// maximum of 12 can be set.
	IntervalMonth *int64 `json:"interval_month,omitempty"`
}

// NewKeyPolicyRotationRotation : Instantiate KeyPolicyRotationRotation (Generic Model Constructor)
func (*IbmKeyProtectApiV2) NewKeyPolicyRotationRotation(enabled bool) (_model *KeyPolicyRotationRotation, err error) {
	_model = &KeyPolicyRotationRotation{
		Enabled: core.BoolPtr(enabled),
	}
	err = core.ValidateStruct(_model, "required parameters")
	if err != nil {
		err = core.SDKErrorf(err, "", "model-missing-required", common.GetComponentInfo())
	}
	return
}

// UnmarshalKeyPolicyRotationRotation unmarshals an instance of KeyPolicyRotationRotation from the specified map of raw messages.
func UnmarshalKeyPolicyRotationRotation(m map[string]json.RawMessage, result interface{}) (err error) {
	obj := new(KeyPolicyRotationRotation)
	err = core.UnmarshalPrimitive(m, "enabled", &obj.Enabled)
	if err != nil {
		err = core.SDKErrorf(err, "", "enabled-error", common.GetComponentInfo())
		return
	}
	err = core.UnmarshalPrimitive(m, "interval_month", &obj.IntervalMonth)
	if err != nil {
		err = core.SDKErrorf(err, "", "interval_month-error", common.GetComponentInfo())
		return
	}
	reflect.ValueOf(result).Elem().Set(reflect.ValueOf(obj))
	return
}

// KeyRing : Base properties of an instance key ring.
type KeyRing struct {
	// An ID that identifies the key ring. Each ID is unique only within the given instance and is not reserved across the
	// Key Protect service.
	ID *string `json:"id,omitempty"`

	// The date the key ring was created. The date format follows RFC 3339.
	CreationDate *strfmt.DateTime `json:"creationDate,omitempty"`

	// The unique identifier for the user that created the key ring.
	CreatedBy *string `json:"createdBy,omitempty"`
}

// UnmarshalKeyRing unmarshals an instance of KeyRing from the specified map of raw messages.
func UnmarshalKeyRing(m map[string]json.RawMessage, result interface{}) (err error) {
	obj := new(KeyRing)
	err = core.UnmarshalPrimitive(m, "id", &obj.ID)
	if err != nil {
		err = core.SDKErrorf(err, "", "id-error", common.GetComponentInfo())
		return
	}
	err = core.UnmarshalPrimitive(m, "creationDate", &obj.CreationDate)
	if err != nil {
		err = core.SDKErrorf(err, "", "creationDate-error", common.GetComponentInfo())
		return
	}
	err = core.UnmarshalPrimitive(m, "createdBy", &obj.CreatedBy)
	if err != nil {
		err = core.SDKErrorf(err, "", "createdBy-error", common.GetComponentInfo())
		return
	}
	reflect.ValueOf(result).Elem().Set(reflect.ValueOf(obj))
	return
}

// KeyVersion : Properties associated with a specific key version.
type KeyVersion struct {
	// The ID of the key version.
	ID *string `json:"id,omitempty"`

	// The date that the version of the key was created.
	CreationDate *strfmt.DateTime `json:"creationDate,omitempty"`
}

// UnmarshalKeyVersion unmarshals an instance of KeyVersion from the specified map of raw messages.
func UnmarshalKeyVersion(m map[string]json.RawMessage, result interface{}) (err error) {
	obj := new(KeyVersion)
	err = core.UnmarshalPrimitive(m, "id", &obj.ID)
	if err != nil {
		err = core.SDKErrorf(err, "", "id-error", common.GetComponentInfo())
		return
	}
	err = core.UnmarshalPrimitive(m, "creationDate", &obj.CreationDate)
	if err != nil {
		err = core.SDKErrorf(err, "", "creationDate-error", common.GetComponentInfo())
		return
	}
	reflect.ValueOf(result).Elem().Set(reflect.ValueOf(obj))
	return
}

// KeyWithPayload : Properties returned only for DELETE.
type KeyWithPayload struct {
	// Specifies the MIME type that represents the key resource. Currently, only the default is supported.
	Type *string `json:"type,omitempty"`

	// The v4 UUID used to uniquely identify the resource, as specified by RFC 4122.
	ID *string `json:"id,omitempty"`

	// A human-readable name assigned to your key for convenience. To protect your privacy do not use personal data, such
	// as your name or location, as the name for your key.
	Name *string `json:"name,omitempty"`

	// One or more, up to a total of five, human-readable unique aliases assigned to your key. To protect your privacy do
	// not use personal data, such as your name or location, as an alias for your key. Each alias must be alphanumeric and
	// cannot contain spaces or special characters other than `-` or `_`. The alias cannot be a UUID and must not be a Key
	// Protect reserved name: `allowed_ip`, `key`, `keys`, `metadata`, `policy`, `policies`, `registration`,
	// `registrations`, `ring`, `rings`, `rotate`, `wrap`, `unwrap`, `rewrap`, `version`, `versions`.
	Aliases []string `json:"aliases,omitempty"`

	// A text field used to provide a more detailed description of the key. The maximum length is 240 characters. To
	// protect your privacy, do not use personal data, such as your name or location, as a description for your key.
	Description *string `json:"description,omitempty"`

	// Up to 30 tags can be created. Tags can be between 0-30 characters, including spaces. Special characters not
	// permitted include angled brackets, comma, colon, ampersand, and vertical pipe character (|). To protect your
	// privacy, do not use personal data, such as your name or location, as a tag for your key.
	Tags []string `json:"tags,omitempty"`

	// The key state based on NIST SP 800-57. States are integers and correspond to the Pre-activation = 0, Active = 1,
	// Suspended = 2, Deactivated = 3, and Destroyed = 5 values.
	State *int64 `json:"state,omitempty"`

	// The date and time that the key expires in the system, in RFC 3339 format (YYYY-MM-DD HH:MM:SS.SS, for example
	// 2019-10-12T07:20:50.52Z). Keys created with an expiration date automatically transition to the Deactivated state
	// within one hour after expiration. In this state, the only allowed actions on the key are unwrap, rewrap, rotate, and
	// delete. Deactivated keys cannot be used to encrypt (wrap) new data, even if rotated while deactivated. Rotation does
	// not reset or extend the expiration date, nor does it allow the date to be changed. It is recommended that any data
	// encrypted with an expiring or expired key be re-encrypted using a new customer root key (CRK) before the original
	// CRK expires, to prevent service disruptions. Deleting and restoring a deactivated key does not move it back to the
	// Active state. If the expirationDate attribute is omitted, the key does not expire.
	ExpirationDate *strfmt.DateTime `json:"expirationDate,omitempty"`

	// A boolean that determines whether the key material can leave the service. If set to `false`, Key Protect designates
	// the key as a nonextractable root key used for `wrap` and `unwrap` actions. If set to `true`, Key Protect designates
	// the key as a standard key that you can store in your apps and services. Once set to `false` it cannot be changed to
	// `true`.
	Extractable *bool `json:"extractable,omitempty"`

	// The Cloud Resource Name (CRN) that uniquely identifies your cloud resources.
	Crn *string `json:"crn,omitempty"`

	// A boolean that shows whether your key was originally imported or generated in Key Protect. The value is set by Key
	// Protect based on how the key material is initially added to the service. A value of `true` indicates that you must
	// provide new key material when it's time to rotate the key. A value of `false` indicates that Key Protect will
	// generate the new key material on a `rotate` operation, as it did in key creation.
	Imported *bool `json:"imported,omitempty"`

	// An ID that identifies the key ring. Each ID is unique only within the given instance and is not reserved across the
	// Key Protect service.
	KeyRingID *string `json:"keyRingID,omitempty"`

	// The date the key material was created. The date format follows RFC 3339.
	CreationDate *strfmt.DateTime `json:"creationDate,omitempty"`

	// The unique identifier for the resource that created the key.
	CreatedBy *string `json:"createdBy,omitempty"`

	// Deprecated.
	// Deprecated: this field is deprecated and may be removed in a future release.
	AlgorithmType *string `json:"algorithmType,omitempty"`

	// Deprecated.
	AlgorithmMetadata *KeyWithPayloadAlgorithmMetadata `json:"algorithmMetadata,omitempty"`

	// Deprecated.
	// Deprecated: this field is deprecated and may be removed in a future release.
	AlgorithmBitSize *int64 `json:"algorithmBitSize,omitempty"`

	// Deprecated.
	// Deprecated: this field is deprecated and may be removed in a future release.
	AlgorithmMode *string `json:"algorithmMode,omitempty"`

	// A code indicating the reason the key is not in the activation state.
	NonactiveStateReason *int64 `json:"nonactiveStateReason,omitempty"`

	// Updates when any part of the key metadata is modified. The date format follows RFC 3339.
	LastUpdateDate *strfmt.DateTime `json:"lastUpdateDate,omitempty"`

	// Updates to show when the key was last rotated. The date format follows RFC 3339.
	LastRotateDate *strfmt.DateTime `json:"lastRotateDate,omitempty"`

	// Properties associated with a specific key version.
	KeyVersion *KeyVersion `json:"keyVersion,omitempty"`

	// Metadata that indicates the status of a dual authorization policy on the key.
	DualAuthDelete *DualAuthKeyMetadata `json:"dualAuthDelete,omitempty"`

	// Metadata that indicates the status of a rotation policy on the key.
	Rotation *RotationKeyMetadata `json:"rotation,omitempty"`

	// A boolean that determines whether the key has been deleted.
	Deleted *bool `json:"deleted,omitempty"`

	// The date the key material was destroyed. The date format follows RFC 3339.
	DeletionDate *strfmt.DateTime `json:"deletionDate,omitempty"`

	// The unique identifier for the resource that deleted the key.
	DeletedBy *string `json:"deletedBy,omitempty"`

	// The date the key will no longer have the ability to be restored.
	RestoreExpirationDate *strfmt.DateTime `json:"restoreExpirationDate,omitempty"`

	// A boolean that specifies if your key has the ability to be restored. A value of `true` indicates that the key can be
	// restored. A value of `false` indicates that the key is unable to be restored.
	RestoreAllowed *bool `json:"restoreAllowed,omitempty"`

	// A boolean that specifies if the key can be purged. A value of `true` indicates that the key can be purged. A value
	// of `false` indicates that the key is within the purge wait period and is not ready to be purged.
	PurgeAllowed *bool `json:"purgeAllowed,omitempty"`

	// The date the key will be ready to be purged.
	PurgeAllowedFrom *strfmt.DateTime `json:"purgeAllowedFrom,omitempty"`

	// The date the deleted key will be automatically purged from Key Protect system.
	PurgeScheduledOn *strfmt.DateTime `json:"purgeScheduledOn,omitempty"`

	// The key material that you can export to external apps or services.
	// **Note:** If the key has been designated as a root key, the system cannot return the key material.
	Payload *[]byte `json:"payload,omitempty"`
}

// Constants associated with the KeyWithPayload.Type property.
// Specifies the MIME type that represents the key resource. Currently, only the default is supported.
const (
	KeyWithPayload_Type_ApplicationVndIbmKmsKeyJSON = "application/vnd.ibm.kms.key+json"
)

// Constants associated with the KeyWithPayload.AlgorithmType property.
// Deprecated.
const (
	KeyWithPayload_AlgorithmType_Aes        = "AES"
	KeyWithPayload_AlgorithmType_Deprecated = "Deprecated"
)

// Constants associated with the KeyWithPayload.AlgorithmMode property.
// Deprecated.
const (
	KeyWithPayload_AlgorithmMode_CbcPad     = "CBC_PAD"
	KeyWithPayload_AlgorithmMode_Deprecated = "Deprecated"
)

// UnmarshalKeyWithPayload unmarshals an instance of KeyWithPayload from the specified map of raw messages.
func UnmarshalKeyWithPayload(m map[string]json.RawMessage, result interface{}) (err error) {
	obj := new(KeyWithPayload)
	err = core.UnmarshalPrimitive(m, "type", &obj.Type)
	if err != nil {
		err = core.SDKErrorf(err, "", "type-error", common.GetComponentInfo())
		return
	}
	err = core.UnmarshalPrimitive(m, "id", &obj.ID)
	if err != nil {
		err = core.SDKErrorf(err, "", "id-error", common.GetComponentInfo())
		return
	}
	err = core.UnmarshalPrimitive(m, "name", &obj.Name)
	if err != nil {
		err = core.SDKErrorf(err, "", "name-error", common.GetComponentInfo())
		return
	}
	err = core.UnmarshalPrimitive(m, "aliases", &obj.Aliases)
	if err != nil {
		err = core.SDKErrorf(err, "", "aliases-error", common.GetComponentInfo())
		return
	}
	err = core.UnmarshalPrimitive(m, "description", &obj.Description)
	if err != nil {
		err = core.SDKErrorf(err, "", "description-error", common.GetComponentInfo())
		return
	}
	err = core.UnmarshalPrimitive(m, "tags", &obj.Tags)
	if err != nil {
		err = core.SDKErrorf(err, "", "tags-error", common.GetComponentInfo())
		return
	}
	err = core.UnmarshalPrimitive(m, "state", &obj.State)
	if err != nil {
		err = core.SDKErrorf(err, "", "state-error", common.GetComponentInfo())
		return
	}
	err = core.UnmarshalPrimitive(m, "expirationDate", &obj.ExpirationDate)
	if err != nil {
		err = core.SDKErrorf(err, "", "expirationDate-error", common.GetComponentInfo())
		return
	}
	err = core.UnmarshalPrimitive(m, "extractable", &obj.Extractable)
	if err != nil {
		err = core.SDKErrorf(err, "", "extractable-error", common.GetComponentInfo())
		return
	}
	err = core.UnmarshalPrimitive(m, "crn", &obj.Crn)
	if err != nil {
		err = core.SDKErrorf(err, "", "crn-error", common.GetComponentInfo())
		return
	}
	err = core.UnmarshalPrimitive(m, "imported", &obj.Imported)
	if err != nil {
		err = core.SDKErrorf(err, "", "imported-error", common.GetComponentInfo())
		return
	}
	err = core.UnmarshalPrimitive(m, "keyRingID", &obj.KeyRingID)
	if err != nil {
		err = core.SDKErrorf(err, "", "keyRingID-error", common.GetComponentInfo())
		return
	}
	err = core.UnmarshalPrimitive(m, "creationDate", &obj.CreationDate)
	if err != nil {
		err = core.SDKErrorf(err, "", "creationDate-error", common.GetComponentInfo())
		return
	}
	err = core.UnmarshalPrimitive(m, "createdBy", &obj.CreatedBy)
	if err != nil {
		err = core.SDKErrorf(err, "", "createdBy-error", common.GetComponentInfo())
		return
	}
	err = core.UnmarshalPrimitive(m, "algorithmType", &obj.AlgorithmType)
	if err != nil {
		err = core.SDKErrorf(err, "", "algorithmType-error", common.GetComponentInfo())
		return
	}
	err = core.UnmarshalModel(m, "algorithmMetadata", &obj.AlgorithmMetadata, UnmarshalKeyWithPayloadAlgorithmMetadata)
	if err != nil {
		err = core.SDKErrorf(err, "", "algorithmMetadata-error", common.GetComponentInfo())
		return
	}
	err = core.UnmarshalPrimitive(m, "algorithmBitSize", &obj.AlgorithmBitSize)
	if err != nil {
		err = core.SDKErrorf(err, "", "algorithmBitSize-error", common.GetComponentInfo())
		return
	}
	err = core.UnmarshalPrimitive(m, "algorithmMode", &obj.AlgorithmMode)
	if err != nil {
		err = core.SDKErrorf(err, "", "algorithmMode-error", common.GetComponentInfo())
		return
	}
	err = core.UnmarshalPrimitive(m, "nonactiveStateReason", &obj.NonactiveStateReason)
	if err != nil {
		err = core.SDKErrorf(err, "", "nonactiveStateReason-error", common.GetComponentInfo())
		return
	}
	err = core.UnmarshalPrimitive(m, "lastUpdateDate", &obj.LastUpdateDate)
	if err != nil {
		err = core.SDKErrorf(err, "", "lastUpdateDate-error", common.GetComponentInfo())
		return
	}
	err = core.UnmarshalPrimitive(m, "lastRotateDate", &obj.LastRotateDate)
	if err != nil {
		err = core.SDKErrorf(err, "", "lastRotateDate-error", common.GetComponentInfo())
		return
	}
	err = core.UnmarshalModel(m, "keyVersion", &obj.KeyVersion, UnmarshalKeyVersion)
	if err != nil {
		err = core.SDKErrorf(err, "", "keyVersion-error", common.GetComponentInfo())
		return
	}
	err = core.UnmarshalModel(m, "dualAuthDelete", &obj.DualAuthDelete, UnmarshalDualAuthKeyMetadata)
	if err != nil {
		err = core.SDKErrorf(err, "", "dualAuthDelete-error", common.GetComponentInfo())
		return
	}
	err = core.UnmarshalModel(m, "rotation", &obj.Rotation, UnmarshalRotationKeyMetadata)
	if err != nil {
		err = core.SDKErrorf(err, "", "rotation-error", common.GetComponentInfo())
		return
	}
	err = core.UnmarshalPrimitive(m, "deleted", &obj.Deleted)
	if err != nil {
		err = core.SDKErrorf(err, "", "deleted-error", common.GetComponentInfo())
		return
	}
	err = core.UnmarshalPrimitive(m, "deletionDate", &obj.DeletionDate)
	if err != nil {
		err = core.SDKErrorf(err, "", "deletionDate-error", common.GetComponentInfo())
		return
	}
	err = core.UnmarshalPrimitive(m, "deletedBy", &obj.DeletedBy)
	if err != nil {
		err = core.SDKErrorf(err, "", "deletedBy-error", common.GetComponentInfo())
		return
	}
	err = core.UnmarshalPrimitive(m, "restoreExpirationDate", &obj.RestoreExpirationDate)
	if err != nil {
		err = core.SDKErrorf(err, "", "restoreExpirationDate-error", common.GetComponentInfo())
		return
	}
	err = core.UnmarshalPrimitive(m, "restoreAllowed", &obj.RestoreAllowed)
	if err != nil {
		err = core.SDKErrorf(err, "", "restoreAllowed-error", common.GetComponentInfo())
		return
	}
	err = core.UnmarshalPrimitive(m, "purgeAllowed", &obj.PurgeAllowed)
	if err != nil {
		err = core.SDKErrorf(err, "", "purgeAllowed-error", common.GetComponentInfo())
		return
	}
	err = core.UnmarshalPrimitive(m, "purgeAllowedFrom", &obj.PurgeAllowedFrom)
	if err != nil {
		err = core.SDKErrorf(err, "", "purgeAllowedFrom-error", common.GetComponentInfo())
		return
	}
	err = core.UnmarshalPrimitive(m, "purgeScheduledOn", &obj.PurgeScheduledOn)
	if err != nil {
		err = core.SDKErrorf(err, "", "purgeScheduledOn-error", common.GetComponentInfo())
		return
	}
	err = core.UnmarshalPrimitive(m, "payload", &obj.Payload)
	if err != nil {
		err = core.SDKErrorf(err, "", "payload-error", common.GetComponentInfo())
		return
	}
	reflect.ValueOf(result).Elem().Set(reflect.ValueOf(obj))
	return
}

// KeyWithPayloadAlgorithmMetadata : Deprecated.
type KeyWithPayloadAlgorithmMetadata struct {
	// Deprecated.
	BitLength *string `json:"bitLength,omitempty"`

	// Deprecated.
	Mode *string `json:"mode,omitempty"`
}

// Constants associated with the KeyWithPayloadAlgorithmMetadata.Mode property.
// Deprecated.
const (
	KeyWithPayloadAlgorithmMetadata_Mode_CbcPad     = "CBC_PAD"
	KeyWithPayloadAlgorithmMetadata_Mode_Deprecated = "Deprecated"
)

// UnmarshalKeyWithPayloadAlgorithmMetadata unmarshals an instance of KeyWithPayloadAlgorithmMetadata from the specified map of raw messages.
func UnmarshalKeyWithPayloadAlgorithmMetadata(m map[string]json.RawMessage, result interface{}) (err error) {
	obj := new(KeyWithPayloadAlgorithmMetadata)
	err = core.UnmarshalPrimitive(m, "bitLength", &obj.BitLength)
	if err != nil {
		err = core.SDKErrorf(err, "", "bitLength-error", common.GetComponentInfo())
		return
	}
	err = core.UnmarshalPrimitive(m, "mode", &obj.Mode)
	if err != nil {
		err = core.SDKErrorf(err, "", "mode-error", common.GetComponentInfo())
		return
	}
	reflect.ValueOf(result).Elem().Set(reflect.ValueOf(obj))
	return
}

// ListCollectionMetadata : ListCollectionMetadata struct
// Models which "extend" this model:
// - ListCollectionMetadataCollectionMetadataWithTotalCount
// - ListCollectionMetadataCollectionMetadata
type ListCollectionMetadata struct {
	// The type of resources in the resource array.
	CollectionType *string `json:"collectionType,omitempty"`

	// The number of elements in the resource array.
	CollectionTotal *int64 `json:"collectionTotal,omitempty"`

	// The total number of elements that match the request, disregarding limit and offset.
	TotalCount *int64 `json:"totalCount,omitempty"`
}

// Constants associated with the ListCollectionMetadata.CollectionType property.
// The type of resources in the resource array.
const (
	ListCollectionMetadata_CollectionType_ApplicationVndIbmKmsAliasJSON                 = "application/vnd.ibm.kms.alias+json"
	ListCollectionMetadata_CollectionType_ApplicationVndIbmKmsAllowedIpMetadataJSON     = "application/vnd.ibm.kms.allowed_ip_metadata+json"
	ListCollectionMetadata_CollectionType_ApplicationVndIbmKmsCrnJSON                   = "application/vnd.ibm.kms.crn+json"
	ListCollectionMetadata_CollectionType_ApplicationVndIbmKmsErrorJSON                 = "application/vnd.ibm.kms.error+json"
	ListCollectionMetadata_CollectionType_ApplicationVndIbmKmsEventAcknowledgeJSON      = "application/vnd.ibm.kms.event_acknowledge+json"
	ListCollectionMetadata_CollectionType_ApplicationVndIbmKmsImportTokenJSON           = "application/vnd.ibm.kms.import_token+json"
	ListCollectionMetadata_CollectionType_ApplicationVndIbmKmsKeyActionJSON             = "application/vnd.ibm.kms.key_action+json"
	ListCollectionMetadata_CollectionType_ApplicationVndIbmKmsKeyJSON                   = "application/vnd.ibm.kms.key+json"
	ListCollectionMetadata_CollectionType_ApplicationVndIbmKmsKeyRingJSON               = "application/vnd.ibm.kms.key_ring+json"
	ListCollectionMetadata_CollectionType_ApplicationVndIbmKmsKmipAdapterJSON           = "application/vnd.ibm.kms.kmip_adapter+json"
	ListCollectionMetadata_CollectionType_ApplicationVndIbmKmsKmipClientCertificateJSON = "application/vnd.ibm.kms.kmip_client_certificate+json"
	ListCollectionMetadata_CollectionType_ApplicationVndIbmKmsKmipObjectJSON            = "application/vnd.ibm.kms.kmip_object+json"
	ListCollectionMetadata_CollectionType_ApplicationVndIbmKmsPolicyJSON                = "application/vnd.ibm.kms.policy+json"
	ListCollectionMetadata_CollectionType_ApplicationVndIbmKmsRegistrationInputJSON     = "application/vnd.ibm.kms.registration_input+json"
	ListCollectionMetadata_CollectionType_ApplicationVndIbmKmsRegistrationJSON          = "application/vnd.ibm.kms.registration+json"
	ListCollectionMetadata_CollectionType_ApplicationVndIbmKmsResourceCrnJSON           = "application/vnd.ibm.kms.resource_crn+json"
)

func (*ListCollectionMetadata) isaListCollectionMetadata() bool {
	return true
}

type ListCollectionMetadataIntf interface {
	isaListCollectionMetadata() bool
}

// UnmarshalListCollectionMetadata unmarshals an instance of ListCollectionMetadata from the specified map of raw messages.
func UnmarshalListCollectionMetadata(m map[string]json.RawMessage, result interface{}) (err error) {
	obj := new(ListCollectionMetadata)
	err = core.UnmarshalPrimitive(m, "collectionType", &obj.CollectionType)
	if err != nil {
		err = core.SDKErrorf(err, "", "collectionType-error", common.GetComponentInfo())
		return
	}
	err = core.UnmarshalPrimitive(m, "collectionTotal", &obj.CollectionTotal)
	if err != nil {
		err = core.SDKErrorf(err, "", "collectionTotal-error", common.GetComponentInfo())
		return
	}
	err = core.UnmarshalPrimitive(m, "totalCount", &obj.TotalCount)
	if err != nil {
		err = core.SDKErrorf(err, "", "totalCount-error", common.GetComponentInfo())
		return
	}
	reflect.ValueOf(result).Elem().Set(reflect.ValueOf(obj))
	return
}

// ListKMIPAdapters : The base schema for listing kmip adapter(s).
type ListKMIPAdapters struct {
	Metadata ListCollectionMetadataIntf `json:"metadata,omitempty"`

	// A collection of resources.
	Resources []KMIPAdapter `json:"resources,omitempty"`
}

// UnmarshalListKMIPAdapters unmarshals an instance of ListKMIPAdapters from the specified map of raw messages.
func UnmarshalListKMIPAdapters(m map[string]json.RawMessage, result interface{}) (err error) {
	obj := new(ListKMIPAdapters)
	err = core.UnmarshalModel(m, "metadata", &obj.Metadata, UnmarshalListCollectionMetadata)
	if err != nil {
		err = core.SDKErrorf(err, "", "metadata-error", common.GetComponentInfo())
		return
	}
	err = core.UnmarshalModel(m, "resources", &obj.Resources, UnmarshalKMIPAdapter)
	if err != nil {
		err = core.SDKErrorf(err, "", "resources-error", common.GetComponentInfo())
		return
	}
	reflect.ValueOf(result).Elem().Set(reflect.ValueOf(obj))
	return
}

// ListKMIPAdaptersWithTotalCount : The base schema for listing kmip adapter with total count.
type ListKMIPAdaptersWithTotalCount struct {
	// The metadata that describes the resource array.
	Metadata *CollectionMetadataWithTotalCount `json:"metadata" validate:"required"`

	// A collection of resources.
	Resources []KMIPAdapter `json:"resources,omitempty"`
}

// UnmarshalListKMIPAdaptersWithTotalCount unmarshals an instance of ListKMIPAdaptersWithTotalCount from the specified map of raw messages.
func UnmarshalListKMIPAdaptersWithTotalCount(m map[string]json.RawMessage, result interface{}) (err error) {
	obj := new(ListKMIPAdaptersWithTotalCount)
	err = core.UnmarshalModel(m, "metadata", &obj.Metadata, UnmarshalCollectionMetadataWithTotalCount)
	if err != nil {
		err = core.SDKErrorf(err, "", "metadata-error", common.GetComponentInfo())
		return
	}
	err = core.UnmarshalModel(m, "resources", &obj.Resources, UnmarshalKMIPAdapter)
	if err != nil {
		err = core.SDKErrorf(err, "", "resources-error", common.GetComponentInfo())
		return
	}
	reflect.ValueOf(result).Elem().Set(reflect.ValueOf(obj))
	return
}

// ListKMIPClientCertificates : The base schema for listing client certificates in a kmip adapter.
type ListKMIPClientCertificates struct {
	Metadata ListCollectionMetadataIntf `json:"metadata,omitempty"`

	// A collection of resources.
	Resources []KMIPClientCertificate `json:"resources,omitempty"`
}

// UnmarshalListKMIPClientCertificates unmarshals an instance of ListKMIPClientCertificates from the specified map of raw messages.
func UnmarshalListKMIPClientCertificates(m map[string]json.RawMessage, result interface{}) (err error) {
	obj := new(ListKMIPClientCertificates)
	err = core.UnmarshalModel(m, "metadata", &obj.Metadata, UnmarshalListCollectionMetadata)
	if err != nil {
		err = core.SDKErrorf(err, "", "metadata-error", common.GetComponentInfo())
		return
	}
	err = core.UnmarshalModel(m, "resources", &obj.Resources, UnmarshalKMIPClientCertificate)
	if err != nil {
		err = core.SDKErrorf(err, "", "resources-error", common.GetComponentInfo())
		return
	}
	reflect.ValueOf(result).Elem().Set(reflect.ValueOf(obj))
	return
}

// ListKMIPObjectsWithTotalCount : The base schema for listing kmip objects in a kmip adapter with total count.
type ListKMIPObjectsWithTotalCount struct {
	// The metadata that describes the resource array.
	Metadata *CollectionMetadataWithTotalCount `json:"metadata" validate:"required"`

	// A collection of resources.
	Resources []KMIPObject `json:"resources,omitempty"`
}

// UnmarshalListKMIPObjectsWithTotalCount unmarshals an instance of ListKMIPObjectsWithTotalCount from the specified map of raw messages.
func UnmarshalListKMIPObjectsWithTotalCount(m map[string]json.RawMessage, result interface{}) (err error) {
	obj := new(ListKMIPObjectsWithTotalCount)
	err = core.UnmarshalModel(m, "metadata", &obj.Metadata, UnmarshalCollectionMetadataWithTotalCount)
	if err != nil {
		err = core.SDKErrorf(err, "", "metadata-error", common.GetComponentInfo())
		return
	}
	err = core.UnmarshalModel(m, "resources", &obj.Resources, UnmarshalKMIPObject)
	if err != nil {
		err = core.SDKErrorf(err, "", "resources-error", common.GetComponentInfo())
		return
	}
	reflect.ValueOf(result).Elem().Set(reflect.ValueOf(obj))
	return
}

// ListKMIPPartialClientCertificatesWithTotalCount : The base schema for listing client certificates in a kmip adapter with total count.
type ListKMIPPartialClientCertificatesWithTotalCount struct {
	// The metadata that describes the resource array.
	Metadata *CollectionMetadataWithTotalCount `json:"metadata" validate:"required"`

	// A collection of resources.
	Resources []KMIPClientPartialCertificate `json:"resources,omitempty"`
}

// UnmarshalListKMIPPartialClientCertificatesWithTotalCount unmarshals an instance of ListKMIPPartialClientCertificatesWithTotalCount from the specified map of raw messages.
func UnmarshalListKMIPPartialClientCertificatesWithTotalCount(m map[string]json.RawMessage, result interface{}) (err error) {
	obj := new(ListKMIPPartialClientCertificatesWithTotalCount)
	err = core.UnmarshalModel(m, "metadata", &obj.Metadata, UnmarshalCollectionMetadataWithTotalCount)
	if err != nil {
		err = core.SDKErrorf(err, "", "metadata-error", common.GetComponentInfo())
		return
	}
	err = core.UnmarshalModel(m, "resources", &obj.Resources, UnmarshalKMIPClientPartialCertificate)
	if err != nil {
		err = core.SDKErrorf(err, "", "resources-error", common.GetComponentInfo())
		return
	}
	reflect.ValueOf(result).Elem().Set(reflect.ValueOf(obj))
	return
}

// ListKeyRingsOptions : The ListKeyRings options.
type ListKeyRingsOptions struct {
	// The IBM Cloud instance ID that identifies your Key Protect service instance.
	BluemixInstance *string `json:"Bluemix-Instance" validate:"required"`

	// The number of key rings to retrieve. By default, `GET /key_rings` returns 100 key rings including the default key
	// ring. To retrieve a different set of key rings, use `limit` with `offset` to page through your available resources.
	// The maximum value for `limit` is 5,000.
	// **Usage:** If you have 20 key rings in your instance, and you want to retrieve only the first 5 key rings, use
	// `../key_rings?limit=5`.
	Limit *int64 `json:"limit,omitempty"`

	// The number of key rings to skip. By specifying `offset`, you retrieve a subset of key rings that starts with the
	// `offset` value. Use `offset` with `limit` to page through your available resources.
	// **Usage:** If you have 20 key rings in your instance, and you want to retrieve keys 10 through 20, use
	// `../keys?offset=10&limit=10`.
	Offset *int64 `json:"offset,omitempty"`

	// If set to `true`, returns `totalCount` in the response metadata for use with pagination. The `totalCount` value
	// returned specifies the total number of key rings that match the request, disregarding limit and offset. The default
	// is set to false.
	// **Usage:** To return the `totalCount` value for use with pagination, use `../key_rings?totalCount=true`.
	TotalCount *bool `json:"totalCount,omitempty"`

	// The v4 UUID used to correlate and track transactions.
	CorrelationID *string `json:"Correlation-Id,omitempty"`

	// Allows users to set headers on API requests.
	Headers map[string]string
}

// NewListKeyRingsOptions : Instantiate ListKeyRingsOptions
func (*IbmKeyProtectApiV2) NewListKeyRingsOptions(bluemixInstance string) *ListKeyRingsOptions {
	return &ListKeyRingsOptions{
		BluemixInstance: core.StringPtr(bluemixInstance),
	}
}

// SetBluemixInstance : Allow user to set BluemixInstance
func (_options *ListKeyRingsOptions) SetBluemixInstance(bluemixInstance string) *ListKeyRingsOptions {
	_options.BluemixInstance = core.StringPtr(bluemixInstance)
	return _options
}

// SetLimit : Allow user to set Limit
func (_options *ListKeyRingsOptions) SetLimit(limit int64) *ListKeyRingsOptions {
	_options.Limit = core.Int64Ptr(limit)
	return _options
}

// SetOffset : Allow user to set Offset
func (_options *ListKeyRingsOptions) SetOffset(offset int64) *ListKeyRingsOptions {
	_options.Offset = core.Int64Ptr(offset)
	return _options
}

// SetTotalCount : Allow user to set TotalCount
func (_options *ListKeyRingsOptions) SetTotalCount(totalCount bool) *ListKeyRingsOptions {
	_options.TotalCount = core.BoolPtr(totalCount)
	return _options
}

// SetCorrelationID : Allow user to set CorrelationID
func (_options *ListKeyRingsOptions) SetCorrelationID(correlationID string) *ListKeyRingsOptions {
	_options.CorrelationID = core.StringPtr(correlationID)
	return _options
}

// SetHeaders : Allow user to set Headers
func (options *ListKeyRingsOptions) SetHeaders(param map[string]string) *ListKeyRingsOptions {
	options.Headers = param
	return options
}

// ListKeyRingsWithTotalCount : The base schema for listing key rings.
type ListKeyRingsWithTotalCount struct {
	// The metadata that describes the resource array.
	Metadata *CollectionMetadataWithTotalCount `json:"metadata" validate:"required"`

	// A collection of resources.
	Resources []KeyRing `json:"resources,omitempty"`
}

// UnmarshalListKeyRingsWithTotalCount unmarshals an instance of ListKeyRingsWithTotalCount from the specified map of raw messages.
func UnmarshalListKeyRingsWithTotalCount(m map[string]json.RawMessage, result interface{}) (err error) {
	obj := new(ListKeyRingsWithTotalCount)
	err = core.UnmarshalModel(m, "metadata", &obj.Metadata, UnmarshalCollectionMetadataWithTotalCount)
	if err != nil {
		err = core.SDKErrorf(err, "", "metadata-error", common.GetComponentInfo())
		return
	}
	err = core.UnmarshalModel(m, "resources", &obj.Resources, UnmarshalKeyRing)
	if err != nil {
		err = core.SDKErrorf(err, "", "resources-error", common.GetComponentInfo())
		return
	}
	reflect.ValueOf(result).Elem().Set(reflect.ValueOf(obj))
	return
}

// ListKeyVersions : Properties associated with a registration response.
type ListKeyVersions struct {
	Metadata ListCollectionMetadataIntf `json:"metadata,omitempty"`

	// An array of resources.
	Resources []KeyVersion `json:"resources,omitempty"`
}

// UnmarshalListKeyVersions unmarshals an instance of ListKeyVersions from the specified map of raw messages.
func UnmarshalListKeyVersions(m map[string]json.RawMessage, result interface{}) (err error) {
	obj := new(ListKeyVersions)
	err = core.UnmarshalModel(m, "metadata", &obj.Metadata, UnmarshalListCollectionMetadata)
	if err != nil {
		err = core.SDKErrorf(err, "", "metadata-error", common.GetComponentInfo())
		return
	}
	err = core.UnmarshalModel(m, "resources", &obj.Resources, UnmarshalKeyVersion)
	if err != nil {
		err = core.SDKErrorf(err, "", "resources-error", common.GetComponentInfo())
		return
	}
	reflect.ValueOf(result).Elem().Set(reflect.ValueOf(obj))
	return
}

// ListKeys : The base schema for listing keys.
type ListKeys struct {
	// The metadata that describes the list keys response.
	Metadata *CollectionMetadataListKeys `json:"metadata" validate:"required"`

	// A collection of resources.
	Resources []KeyFullRepresentation `json:"resources,omitempty"`
}

// UnmarshalListKeys unmarshals an instance of ListKeys from the specified map of raw messages.
func UnmarshalListKeys(m map[string]json.RawMessage, result interface{}) (err error) {
	obj := new(ListKeys)
	err = core.UnmarshalModel(m, "metadata", &obj.Metadata, UnmarshalCollectionMetadataListKeys)
	if err != nil {
		err = core.SDKErrorf(err, "", "metadata-error", common.GetComponentInfo())
		return
	}
	err = core.UnmarshalModel(m, "resources", &obj.Resources, UnmarshalKeyFullRepresentation)
	if err != nil {
		err = core.SDKErrorf(err, "", "resources-error", common.GetComponentInfo())
		return
	}
	reflect.ValueOf(result).Elem().Set(reflect.ValueOf(obj))
	return
}

// ListKeysMetadataPropertiesSearchQuery : Represents the parsed search query used for matching logic. Only returned when a search is requested.
type ListKeysMetadataPropertiesSearchQuery struct {
	// final string to use for matching logic.
	Query *string `json:"query" validate:"required"`

	// list of scopes to search in.
	Scopes []string `json:"scopes" validate:"required"`

	// invert matching logic.
	Not *bool `json:"not,omitempty"`

	// only match query strings that are fully identical (case insensitive).
	Exact *bool `json:"exact,omitempty"`
}

// Constants associated with the ListKeysMetadataPropertiesSearchQuery.Scopes property.
const (
	ListKeysMetadataPropertiesSearchQuery_Scopes_Alias = "alias"
	ListKeysMetadataPropertiesSearchQuery_Scopes_Name  = "name"
)

// UnmarshalListKeysMetadataPropertiesSearchQuery unmarshals an instance of ListKeysMetadataPropertiesSearchQuery from the specified map of raw messages.
func UnmarshalListKeysMetadataPropertiesSearchQuery(m map[string]json.RawMessage, result interface{}) (err error) {
	obj := new(ListKeysMetadataPropertiesSearchQuery)
	err = core.UnmarshalPrimitive(m, "query", &obj.Query)
	if err != nil {
		err = core.SDKErrorf(err, "", "query-error", common.GetComponentInfo())
		return
	}
	err = core.UnmarshalPrimitive(m, "scopes", &obj.Scopes)
	if err != nil {
		err = core.SDKErrorf(err, "", "scopes-error", common.GetComponentInfo())
		return
	}
	err = core.UnmarshalPrimitive(m, "not", &obj.Not)
	if err != nil {
		err = core.SDKErrorf(err, "", "not-error", common.GetComponentInfo())
		return
	}
	err = core.UnmarshalPrimitive(m, "exact", &obj.Exact)
	if err != nil {
		err = core.SDKErrorf(err, "", "exact-error", common.GetComponentInfo())
		return
	}
	reflect.ValueOf(result).Elem().Set(reflect.ValueOf(obj))
	return
}

// MetricsProperties : User defined metadata that is associated with the `metrics` instance policy type.
type MetricsProperties struct {
	// If set to `true`, Key Protect will send service instance metrics to your [Cloud Monitoring With
	// Sysdig](/docs/Monitoring-with-Sysdig?topic=Monitoring-with-Sysdig-getting-started) monitoring instance. By default,
	// sending metrics to your [Cloud Monitoring With
	// Sysdig](/docs/Monitoring-with-Sysdig?topic=Monitoring-with-Sysdig-getting-started) monitoring instance is disabled.
	// **Note:** A metrics policy will add an additional metrics source to your [Cloud Monitoring With
	// Sysdig](/docs/Monitoring-with-Sysdig?topic=Monitoring-with-Sysdig-getting-started) monitoring instance. For more
	// information, see [Enabling Platform
	// Metrics](/docs/Monitoring-with-Sysdig?topic=Monitoring-with-Sysdig-platform_metrics_enabling) for more information.
	Enabled *bool `json:"enabled" validate:"required"`
}

// NewMetricsProperties : Instantiate MetricsProperties (Generic Model Constructor)
func (*IbmKeyProtectApiV2) NewMetricsProperties(enabled bool) (_model *MetricsProperties, err error) {
	_model = &MetricsProperties{
		Enabled: core.BoolPtr(enabled),
	}
	err = core.ValidateStruct(_model, "required parameters")
	if err != nil {
		err = core.SDKErrorf(err, "", "model-missing-required", common.GetComponentInfo())
	}
	return
}

// UnmarshalMetricsProperties unmarshals an instance of MetricsProperties from the specified map of raw messages.
func UnmarshalMetricsProperties(m map[string]json.RawMessage, result interface{}) (err error) {
	obj := new(MetricsProperties)
	err = core.UnmarshalPrimitive(m, "enabled", &obj.Enabled)
	if err != nil {
		err = core.SDKErrorf(err, "", "enabled-error", common.GetComponentInfo())
		return
	}
	reflect.ValueOf(result).Elem().Set(reflect.ValueOf(obj))
	return
}

// PatchKeyOptions : The PatchKey options.
type PatchKeyOptions struct {
	// The v4 UUID that uniquely identifies the key.
	ID *string `json:"id" validate:"required,ne="`

	// The IBM Cloud instance ID that identifies your Key Protect service instance.
	BluemixInstance *string `json:"Bluemix-Instance" validate:"required"`

	// The base request for patch key.
	KeyPatchBody io.ReadCloser `json:"KeyPatchBody,omitempty"`

	// The v4 UUID used to correlate and track transactions.
	CorrelationID *string `json:"Correlation-Id,omitempty"`

	// The ID of the key ring that the specified key is a part of. When the header is not specified, Key Protect will
	// perform a key ring lookup. For a more optimized request, specify the key ring on every call. The key ring ID of keys
	// that are created without an `X-Kms-Key-Ring` header is: `default`.
	XKmsKeyRing *string `json:"X-Kms-Key-Ring,omitempty"`

	// Allows users to set headers on API requests.
	Headers map[string]string
}

// NewPatchKeyOptions : Instantiate PatchKeyOptions
func (*IbmKeyProtectApiV2) NewPatchKeyOptions(id string, bluemixInstance string) *PatchKeyOptions {
	return &PatchKeyOptions{
		ID:              core.StringPtr(id),
		BluemixInstance: core.StringPtr(bluemixInstance),
	}
}

// SetID : Allow user to set ID
func (_options *PatchKeyOptions) SetID(id string) *PatchKeyOptions {
	_options.ID = core.StringPtr(id)
	return _options
}

// SetBluemixInstance : Allow user to set BluemixInstance
func (_options *PatchKeyOptions) SetBluemixInstance(bluemixInstance string) *PatchKeyOptions {
	_options.BluemixInstance = core.StringPtr(bluemixInstance)
	return _options
}

// SetKeyPatchBody : Allow user to set KeyPatchBody
func (_options *PatchKeyOptions) SetKeyPatchBody(keyPatchBody io.ReadCloser) *PatchKeyOptions {
	_options.KeyPatchBody = keyPatchBody
	return _options
}

// SetCorrelationID : Allow user to set CorrelationID
func (_options *PatchKeyOptions) SetCorrelationID(correlationID string) *PatchKeyOptions {
	_options.CorrelationID = core.StringPtr(correlationID)
	return _options
}

// SetXKmsKeyRing : Allow user to set XKmsKeyRing
func (_options *PatchKeyOptions) SetXKmsKeyRing(xKmsKeyRing string) *PatchKeyOptions {
	_options.XKmsKeyRing = core.StringPtr(xKmsKeyRing)
	return _options
}

// SetHeaders : Allow user to set Headers
func (options *PatchKeyOptions) SetHeaders(param map[string]string) *PatchKeyOptions {
	options.Headers = param
	return options
}

// PatchKeyResponseBody : The base schema for patch key response body.
type PatchKeyResponseBody struct {
	// The metadata that describes the resource array.
	Metadata *CollectionMetadata `json:"metadata,omitempty"`

	// An array of resources.
	Resources []KeyFullRepresentation `json:"resources,omitempty"`
}

// UnmarshalPatchKeyResponseBody unmarshals an instance of PatchKeyResponseBody from the specified map of raw messages.
func UnmarshalPatchKeyResponseBody(m map[string]json.RawMessage, result interface{}) (err error) {
	obj := new(PatchKeyResponseBody)
	err = core.UnmarshalModel(m, "metadata", &obj.Metadata, UnmarshalCollectionMetadata)
	if err != nil {
		err = core.SDKErrorf(err, "", "metadata-error", common.GetComponentInfo())
		return
	}
	err = core.UnmarshalModel(m, "resources", &obj.Resources, UnmarshalKeyFullRepresentation)
	if err != nil {
		err = core.SDKErrorf(err, "", "resources-error", common.GetComponentInfo())
		return
	}
	reflect.ValueOf(result).Elem().Set(reflect.ValueOf(obj))
	return
}

// PostImportTokenOptions : The PostImportToken options.
type PostImportTokenOptions struct {
	// The IBM Cloud instance ID that identifies your Key Protect service instance.
	BluemixInstance *string `json:"Bluemix-Instance" validate:"required"`

	// The time in seconds from the creation of an import token that determines how long its associated public key remains
	// valid. The minimum value is `300` seconds (5 minutes), and the maximum value is `86400` (24 hours). The default
	// value is `600` (10 minutes).
	Expiration *float64 `json:"expiration,omitempty"`

	// The number of times that an import token can be retrieved within its expiration time before it is no longer
	// accessible.
	MaxAllowedRetrievals *float64 `json:"maxAllowedRetrievals,omitempty"`

	// The v4 UUID used to correlate and track transactions.
	CorrelationID *string `json:"Correlation-Id,omitempty"`

	// The ID of the key ring that the specified key belongs to. When the header is not specified, Key Protect will perform
	// a key ring lookup. For a more optimized request, specify the key ring on every call. The key ring ID of keys that
	// are created without an `X-Kms-Key-Ring` header is: `default`.
	XKmsKeyRing *string `json:"X-Kms-Key-Ring,omitempty"`

	// Allows users to set headers on API requests.
	Headers map[string]string
}

// NewPostImportTokenOptions : Instantiate PostImportTokenOptions
func (*IbmKeyProtectApiV2) NewPostImportTokenOptions(bluemixInstance string) *PostImportTokenOptions {
	return &PostImportTokenOptions{
		BluemixInstance: core.StringPtr(bluemixInstance),
	}
}

// SetBluemixInstance : Allow user to set BluemixInstance
func (_options *PostImportTokenOptions) SetBluemixInstance(bluemixInstance string) *PostImportTokenOptions {
	_options.BluemixInstance = core.StringPtr(bluemixInstance)
	return _options
}

// SetExpiration : Allow user to set Expiration
func (_options *PostImportTokenOptions) SetExpiration(expiration float64) *PostImportTokenOptions {
	_options.Expiration = core.Float64Ptr(expiration)
	return _options
}

// SetMaxAllowedRetrievals : Allow user to set MaxAllowedRetrievals
func (_options *PostImportTokenOptions) SetMaxAllowedRetrievals(maxAllowedRetrievals float64) *PostImportTokenOptions {
	_options.MaxAllowedRetrievals = core.Float64Ptr(maxAllowedRetrievals)
	return _options
}

// SetCorrelationID : Allow user to set CorrelationID
func (_options *PostImportTokenOptions) SetCorrelationID(correlationID string) *PostImportTokenOptions {
	_options.CorrelationID = core.StringPtr(correlationID)
	return _options
}

// SetXKmsKeyRing : Allow user to set XKmsKeyRing
func (_options *PostImportTokenOptions) SetXKmsKeyRing(xKmsKeyRing string) *PostImportTokenOptions {
	_options.XKmsKeyRing = core.StringPtr(xKmsKeyRing)
	return _options
}

// SetHeaders : Allow user to set Headers
func (options *PostImportTokenOptions) SetHeaders(param map[string]string) *PostImportTokenOptions {
	options.Headers = param
	return options
}

// PurgeKey : The base schema for purged key.
type PurgeKey struct {
	// The metadata that describes the resource array.
	Metadata *CollectionMetadata `json:"metadata" validate:"required"`

	// A collection of resources.
	Resources []KeyFullRepresentation `json:"resources" validate:"required"`
}

// UnmarshalPurgeKey unmarshals an instance of PurgeKey from the specified map of raw messages.
func UnmarshalPurgeKey(m map[string]json.RawMessage, result interface{}) (err error) {
	obj := new(PurgeKey)
	err = core.UnmarshalModel(m, "metadata", &obj.Metadata, UnmarshalCollectionMetadata)
	if err != nil {
		err = core.SDKErrorf(err, "", "metadata-error", common.GetComponentInfo())
		return
	}
	err = core.UnmarshalModel(m, "resources", &obj.Resources, UnmarshalKeyFullRepresentation)
	if err != nil {
		err = core.SDKErrorf(err, "", "resources-error", common.GetComponentInfo())
		return
	}
	reflect.ValueOf(result).Elem().Set(reflect.ValueOf(obj))
	return
}

// PurgeKeyOptions : The PurgeKey options.
type PurgeKeyOptions struct {
	// The v4 UUID or alias that uniquely identifies the key.
	ID *string `json:"id" validate:"required,ne="`

	// The IBM Cloud instance ID that identifies your Key Protect service instance.
	BluemixInstance *string `json:"Bluemix-Instance" validate:"required"`

	// The v4 UUID used to correlate and track transactions.
	CorrelationID *string `json:"Correlation-Id,omitempty"`

	// The ID of the key ring that the specified key is a part of. When the header is not specified, Key Protect will
	// perform a key ring lookup. For a more optimized request, specify the key ring on every call. The key ring ID of keys
	// that are created without an `X-Kms-Key-Ring` header is: `default`.
	XKmsKeyRing *string `json:"X-Kms-Key-Ring,omitempty"`

	// Alters server behavior for POST or DELETE operations. A header with `return=minimal` causes the service to return
	// only the key identifier as metadata. A header containing `return=representation` returns both the key material and
	// metadata in the response entity-body. If the key has been designated as a root key, the system cannot return the key
	// material.
	// **Note:** During POST operations, Key Protect may not immediately return the key material due to key generation
	// time. To retrieve the key material, you can perform a subsequent `GET /keys/{id}` request.
	Prefer *string `json:"Prefer,omitempty"`

	// Allows users to set headers on API requests.
	Headers map[string]string
}

// Constants associated with the PurgeKeyOptions.Prefer property.
// Alters server behavior for POST or DELETE operations. A header with `return=minimal` causes the service to return
// only the key identifier as metadata. A header containing `return=representation` returns both the key material and
// metadata in the response entity-body. If the key has been designated as a root key, the system cannot return the key
// material.
// **Note:** During POST operations, Key Protect may not immediately return the key material due to key generation time.
// To retrieve the key material, you can perform a subsequent `GET /keys/{id}` request.
const (
	PurgeKeyOptions_Prefer_ReturnMinimal        = "return=minimal"
	PurgeKeyOptions_Prefer_ReturnRepresentation = "return=representation"
)

// NewPurgeKeyOptions : Instantiate PurgeKeyOptions
func (*IbmKeyProtectApiV2) NewPurgeKeyOptions(id string, bluemixInstance string) *PurgeKeyOptions {
	return &PurgeKeyOptions{
		ID:              core.StringPtr(id),
		BluemixInstance: core.StringPtr(bluemixInstance),
	}
}

// SetID : Allow user to set ID
func (_options *PurgeKeyOptions) SetID(id string) *PurgeKeyOptions {
	_options.ID = core.StringPtr(id)
	return _options
}

// SetBluemixInstance : Allow user to set BluemixInstance
func (_options *PurgeKeyOptions) SetBluemixInstance(bluemixInstance string) *PurgeKeyOptions {
	_options.BluemixInstance = core.StringPtr(bluemixInstance)
	return _options
}

// SetCorrelationID : Allow user to set CorrelationID
func (_options *PurgeKeyOptions) SetCorrelationID(correlationID string) *PurgeKeyOptions {
	_options.CorrelationID = core.StringPtr(correlationID)
	return _options
}

// SetXKmsKeyRing : Allow user to set XKmsKeyRing
func (_options *PurgeKeyOptions) SetXKmsKeyRing(xKmsKeyRing string) *PurgeKeyOptions {
	_options.XKmsKeyRing = core.StringPtr(xKmsKeyRing)
	return _options
}

// SetPrefer : Allow user to set Prefer
func (_options *PurgeKeyOptions) SetPrefer(prefer string) *PurgeKeyOptions {
	_options.Prefer = core.StringPtr(prefer)
	return _options
}

// SetHeaders : Allow user to set Headers
func (options *PurgeKeyOptions) SetHeaders(param map[string]string) *PurgeKeyOptions {
	options.Headers = param
	return options
}

// PutInstancePolicyOptions : The PutInstancePolicy options.
type PutInstancePolicyOptions struct {
	// The IBM Cloud instance ID that identifies your Key Protect service instance.
	BluemixInstance *string `json:"Bluemix-Instance" validate:"required"`

	// The base request for the create or update of instance level policies.
	InstancePolicyPutBody SetInstancePoliciesOneOfIntf `json:"InstancePolicyPutBody" validate:"required"`

	// The v4 UUID used to correlate and track transactions.
	CorrelationID *string `json:"Correlation-Id,omitempty"`

	// The type of policy that is associated with the specified instance.
	Policy *string `json:"policy,omitempty"`

	// Allows users to set headers on API requests.
	Headers map[string]string
}

// Constants associated with the PutInstancePolicyOptions.Policy property.
// The type of policy that is associated with the specified instance.
const (
	PutInstancePolicyOptions_Policy_Allowedip             = "allowedIP"
	PutInstancePolicyOptions_Policy_Allowednetwork        = "allowedNetwork"
	PutInstancePolicyOptions_Policy_Dualauthdelete        = "dualAuthDelete"
	PutInstancePolicyOptions_Policy_Keycreateimportaccess = "keyCreateImportAccess"
	PutInstancePolicyOptions_Policy_Metrics               = "metrics"
	PutInstancePolicyOptions_Policy_Rotation              = "rotation"
)

// NewPutInstancePolicyOptions : Instantiate PutInstancePolicyOptions
func (*IbmKeyProtectApiV2) NewPutInstancePolicyOptions(bluemixInstance string, instancePolicyPutBody SetInstancePoliciesOneOfIntf) *PutInstancePolicyOptions {
	return &PutInstancePolicyOptions{
		BluemixInstance:       core.StringPtr(bluemixInstance),
		InstancePolicyPutBody: instancePolicyPutBody,
	}
}

// SetBluemixInstance : Allow user to set BluemixInstance
func (_options *PutInstancePolicyOptions) SetBluemixInstance(bluemixInstance string) *PutInstancePolicyOptions {
	_options.BluemixInstance = core.StringPtr(bluemixInstance)
	return _options
}

// SetInstancePolicyPutBody : Allow user to set InstancePolicyPutBody
func (_options *PutInstancePolicyOptions) SetInstancePolicyPutBody(instancePolicyPutBody SetInstancePoliciesOneOfIntf) *PutInstancePolicyOptions {
	_options.InstancePolicyPutBody = instancePolicyPutBody
	return _options
}

// SetCorrelationID : Allow user to set CorrelationID
func (_options *PutInstancePolicyOptions) SetCorrelationID(correlationID string) *PutInstancePolicyOptions {
	_options.CorrelationID = core.StringPtr(correlationID)
	return _options
}

// SetPolicy : Allow user to set Policy
func (_options *PutInstancePolicyOptions) SetPolicy(policy string) *PutInstancePolicyOptions {
	_options.Policy = core.StringPtr(policy)
	return _options
}

// SetHeaders : Allow user to set Headers
func (options *PutInstancePolicyOptions) SetHeaders(param map[string]string) *PutInstancePolicyOptions {
	options.Headers = param
	return options
}

// PutPolicyOptions : The PutPolicy options.
type PutPolicyOptions struct {
	// The v4 UUID or alias that uniquely identifies the key.
	ID *string `json:"id" validate:"required,ne="`

	// The IBM Cloud instance ID that identifies your Key Protect service instance.
	BluemixInstance *string `json:"Bluemix-Instance" validate:"required"`

	// The base request for key policy create or update.
	KeyPolicyPutBody SetKeyPoliciesOneOfIntf `json:"KeyPolicyPutBody" validate:"required"`

	// The v4 UUID used to correlate and track transactions.
	CorrelationID *string `json:"Correlation-Id,omitempty"`

	// The ID of the key ring that the specified key is a part of. When the header is not specified, Key Protect will
	// perform a key ring lookup. For a more optimized request, specify the key ring on every call. The key ring ID of keys
	// that are created without an `X-Kms-Key-Ring` header is: `default`.
	XKmsKeyRing *string `json:"X-Kms-Key-Ring,omitempty"`

	// The type of policy that is associated with the specified key.
	Policy *string `json:"policy,omitempty"`

	// Allows users to set headers on API requests.
	Headers map[string]string
}

// Constants associated with the PutPolicyOptions.Policy property.
// The type of policy that is associated with the specified key.
const (
	PutPolicyOptions_Policy_Dualauthdelete = "dualAuthDelete"
	PutPolicyOptions_Policy_Rotation       = "rotation"
)

// NewPutPolicyOptions : Instantiate PutPolicyOptions
func (*IbmKeyProtectApiV2) NewPutPolicyOptions(id string, bluemixInstance string, keyPolicyPutBody SetKeyPoliciesOneOfIntf) *PutPolicyOptions {
	return &PutPolicyOptions{
		ID:               core.StringPtr(id),
		BluemixInstance:  core.StringPtr(bluemixInstance),
		KeyPolicyPutBody: keyPolicyPutBody,
	}
}

// SetID : Allow user to set ID
func (_options *PutPolicyOptions) SetID(id string) *PutPolicyOptions {
	_options.ID = core.StringPtr(id)
	return _options
}

// SetBluemixInstance : Allow user to set BluemixInstance
func (_options *PutPolicyOptions) SetBluemixInstance(bluemixInstance string) *PutPolicyOptions {
	_options.BluemixInstance = core.StringPtr(bluemixInstance)
	return _options
}

// SetKeyPolicyPutBody : Allow user to set KeyPolicyPutBody
func (_options *PutPolicyOptions) SetKeyPolicyPutBody(keyPolicyPutBody SetKeyPoliciesOneOfIntf) *PutPolicyOptions {
	_options.KeyPolicyPutBody = keyPolicyPutBody
	return _options
}

// SetCorrelationID : Allow user to set CorrelationID
func (_options *PutPolicyOptions) SetCorrelationID(correlationID string) *PutPolicyOptions {
	_options.CorrelationID = core.StringPtr(correlationID)
	return _options
}

// SetXKmsKeyRing : Allow user to set XKmsKeyRing
func (_options *PutPolicyOptions) SetXKmsKeyRing(xKmsKeyRing string) *PutPolicyOptions {
	_options.XKmsKeyRing = core.StringPtr(xKmsKeyRing)
	return _options
}

// SetPolicy : Allow user to set Policy
func (_options *PutPolicyOptions) SetPolicy(policy string) *PutPolicyOptions {
	_options.Policy = core.StringPtr(policy)
	return _options
}

// SetHeaders : Allow user to set Headers
func (options *PutPolicyOptions) SetHeaders(param map[string]string) *PutPolicyOptions {
	options.Headers = param
	return options
}

// RegistrationResource : Properties associated with a registration.
type RegistrationResource struct {
	// The ID that identifies the root key that is associated with the specified cloud resource.
	KeyID *string `json:"keyId,omitempty"`

	// The human-readable reference assigned to the key that is associated with the specified cloud resource.
	KeyName *string `json:"keyName,omitempty"`

	// The [Cloud Resource Name](/docs/account?topic=account-crn) (CRN) that represents the cloud resource, such as a Cloud
	// Object Storage bucket, that is associated with the key.
	ResourceCrn *string `json:"resourceCrn,omitempty"`

	// The unique identifier for the resource that created the registration.
	CreatedBy *string `json:"createdBy,omitempty"`

	// The date the registration was created. The date format follows RFC 3339.
	CreationDate *strfmt.DateTime `json:"creationDate,omitempty"`

	// The unique identifier for the resource that updated the registration.
	UpdatedBy *string `json:"updatedBy,omitempty"`

	// Updates when the registration is modified. The date format follows RFC 3339.
	LastUpdated *strfmt.DateTime `json:"lastUpdated,omitempty"`

	// Description of the purpose of the registration.
	Description *string `json:"description,omitempty"`

	// A boolean that determines whether Key Protect must prevent deletion of a root key.
	PreventKeyDeletion *bool `json:"preventKeyDeletion,omitempty"`

	// Properties associated with a specific key version.
	KeyVersion *KeyVersion `json:"keyVersion,omitempty"`
}

// UnmarshalRegistrationResource unmarshals an instance of RegistrationResource from the specified map of raw messages.
func UnmarshalRegistrationResource(m map[string]json.RawMessage, result interface{}) (err error) {
	obj := new(RegistrationResource)
	err = core.UnmarshalPrimitive(m, "keyId", &obj.KeyID)
	if err != nil {
		err = core.SDKErrorf(err, "", "keyId-error", common.GetComponentInfo())
		return
	}
	err = core.UnmarshalPrimitive(m, "keyName", &obj.KeyName)
	if err != nil {
		err = core.SDKErrorf(err, "", "keyName-error", common.GetComponentInfo())
		return
	}
	err = core.UnmarshalPrimitive(m, "resourceCrn", &obj.ResourceCrn)
	if err != nil {
		err = core.SDKErrorf(err, "", "resourceCrn-error", common.GetComponentInfo())
		return
	}
	err = core.UnmarshalPrimitive(m, "createdBy", &obj.CreatedBy)
	if err != nil {
		err = core.SDKErrorf(err, "", "createdBy-error", common.GetComponentInfo())
		return
	}
	err = core.UnmarshalPrimitive(m, "creationDate", &obj.CreationDate)
	if err != nil {
		err = core.SDKErrorf(err, "", "creationDate-error", common.GetComponentInfo())
		return
	}
	err = core.UnmarshalPrimitive(m, "updatedBy", &obj.UpdatedBy)
	if err != nil {
		err = core.SDKErrorf(err, "", "updatedBy-error", common.GetComponentInfo())
		return
	}
	err = core.UnmarshalPrimitive(m, "lastUpdated", &obj.LastUpdated)
	if err != nil {
		err = core.SDKErrorf(err, "", "lastUpdated-error", common.GetComponentInfo())
		return
	}
	err = core.UnmarshalPrimitive(m, "description", &obj.Description)
	if err != nil {
		err = core.SDKErrorf(err, "", "description-error", common.GetComponentInfo())
		return
	}
	err = core.UnmarshalPrimitive(m, "preventKeyDeletion", &obj.PreventKeyDeletion)
	if err != nil {
		err = core.SDKErrorf(err, "", "preventKeyDeletion-error", common.GetComponentInfo())
		return
	}
	err = core.UnmarshalModel(m, "keyVersion", &obj.KeyVersion, UnmarshalKeyVersion)
	if err != nil {
		err = core.SDKErrorf(err, "", "keyVersion-error", common.GetComponentInfo())
		return
	}
	reflect.ValueOf(result).Elem().Set(reflect.ValueOf(obj))
	return
}

// RegistrationWithTotalCount : Properties associated with a list registration response which may include total registration count.
type RegistrationWithTotalCount struct {
	// The metadata that describes the resource array.
	Metadata *CollectionMetadataWithTotalCount `json:"metadata,omitempty"`

	// A collection of resources.
	Resources []RegistrationResource `json:"resources,omitempty"`
}

// UnmarshalRegistrationWithTotalCount unmarshals an instance of RegistrationWithTotalCount from the specified map of raw messages.
func UnmarshalRegistrationWithTotalCount(m map[string]json.RawMessage, result interface{}) (err error) {
	obj := new(RegistrationWithTotalCount)
	err = core.UnmarshalModel(m, "metadata", &obj.Metadata, UnmarshalCollectionMetadataWithTotalCount)
	if err != nil {
		err = core.SDKErrorf(err, "", "metadata-error", common.GetComponentInfo())
		return
	}
	err = core.UnmarshalModel(m, "resources", &obj.Resources, UnmarshalRegistrationResource)
	if err != nil {
		err = core.SDKErrorf(err, "", "resources-error", common.GetComponentInfo())
		return
	}
	reflect.ValueOf(result).Elem().Set(reflect.ValueOf(obj))
	return
}

// RestoreKeyOptions : The RestoreKey options.
type RestoreKeyOptions struct {
	// The v4 UUID or alias that uniquely identifies the key.
	ID *string `json:"id" validate:"required,ne="`

	// The IBM Cloud instance ID that identifies your Key Protect service instance.
	BluemixInstance *string `json:"Bluemix-Instance" validate:"required"`

	// The v4 UUID used to correlate and track transactions.
	CorrelationID *string `json:"Correlation-Id,omitempty"`

	// The ID of the key ring that the specified key is a part of. When the header is not specified, Key Protect will
	// perform a key ring lookup. For a more optimized request, specify the key ring on every call. The key ring ID of keys
	// that are created without an `X-Kms-Key-Ring` header is: `default`.
	XKmsKeyRing *string `json:"X-Kms-Key-Ring,omitempty"`

	// Alters server behavior for POST or DELETE operations. A header with `return=minimal` causes the service to return
	// only the key identifier as metadata. A header containing `return=representation` returns both the key material and
	// metadata in the response entity-body. If the key has been designated as a root key, the system cannot return the key
	// material.
	// **Note:** During POST operations, Key Protect may not immediately return the key material due to key generation
	// time. To retrieve the key material, you can perform a subsequent `GET /keys/{id}` request.
	Prefer *string `json:"Prefer,omitempty"`

	// Allows users to set headers on API requests.
	Headers map[string]string
}

// Constants associated with the RestoreKeyOptions.Prefer property.
// Alters server behavior for POST or DELETE operations. A header with `return=minimal` causes the service to return
// only the key identifier as metadata. A header containing `return=representation` returns both the key material and
// metadata in the response entity-body. If the key has been designated as a root key, the system cannot return the key
// material.
// **Note:** During POST operations, Key Protect may not immediately return the key material due to key generation time.
// To retrieve the key material, you can perform a subsequent `GET /keys/{id}` request.
const (
	RestoreKeyOptions_Prefer_ReturnMinimal        = "return=minimal"
	RestoreKeyOptions_Prefer_ReturnRepresentation = "return=representation"
)

// NewRestoreKeyOptions : Instantiate RestoreKeyOptions
func (*IbmKeyProtectApiV2) NewRestoreKeyOptions(id string, bluemixInstance string) *RestoreKeyOptions {
	return &RestoreKeyOptions{
		ID:              core.StringPtr(id),
		BluemixInstance: core.StringPtr(bluemixInstance),
	}
}

// SetID : Allow user to set ID
func (_options *RestoreKeyOptions) SetID(id string) *RestoreKeyOptions {
	_options.ID = core.StringPtr(id)
	return _options
}

// SetBluemixInstance : Allow user to set BluemixInstance
func (_options *RestoreKeyOptions) SetBluemixInstance(bluemixInstance string) *RestoreKeyOptions {
	_options.BluemixInstance = core.StringPtr(bluemixInstance)
	return _options
}

// SetCorrelationID : Allow user to set CorrelationID
func (_options *RestoreKeyOptions) SetCorrelationID(correlationID string) *RestoreKeyOptions {
	_options.CorrelationID = core.StringPtr(correlationID)
	return _options
}

// SetXKmsKeyRing : Allow user to set XKmsKeyRing
func (_options *RestoreKeyOptions) SetXKmsKeyRing(xKmsKeyRing string) *RestoreKeyOptions {
	_options.XKmsKeyRing = core.StringPtr(xKmsKeyRing)
	return _options
}

// SetPrefer : Allow user to set Prefer
func (_options *RestoreKeyOptions) SetPrefer(prefer string) *RestoreKeyOptions {
	_options.Prefer = core.StringPtr(prefer)
	return _options
}

// SetHeaders : Allow user to set Headers
func (options *RestoreKeyOptions) SetHeaders(param map[string]string) *RestoreKeyOptions {
	options.Headers = param
	return options
}

// RewrapKeyOptions : The RewrapKey options.
type RewrapKeyOptions struct {
	// The v4 UUID or alias that uniquely identifies the key.
	ID *string `json:"id" validate:"required,ne="`

	// The IBM Cloud instance ID that identifies your Key Protect service instance.
	BluemixInstance *string `json:"Bluemix-Instance" validate:"required"`

	// The base request for rewrap key action.
	KeyActionRewrapBody io.ReadCloser `json:"KeyActionRewrapBody" validate:"required"`

	// The v4 UUID used to correlate and track transactions.
	CorrelationID *string `json:"Correlation-Id,omitempty"`

	// The ID of the key ring that the specified key is a part of. When the header is not specified, Key Protect will
	// perform a key ring lookup. For a more optimized request, specify the key ring on every call. The key ring ID of keys
	// that are created without an `X-Kms-Key-Ring` header is: `default`.
	XKmsKeyRing *string `json:"X-Kms-Key-Ring,omitempty"`

	// Allows users to set headers on API requests.
	Headers map[string]string
}

// NewRewrapKeyOptions : Instantiate RewrapKeyOptions
func (*IbmKeyProtectApiV2) NewRewrapKeyOptions(id string, bluemixInstance string, keyActionRewrapBody io.ReadCloser) *RewrapKeyOptions {
	return &RewrapKeyOptions{
		ID:                  core.StringPtr(id),
		BluemixInstance:     core.StringPtr(bluemixInstance),
		KeyActionRewrapBody: keyActionRewrapBody,
	}
}

// SetID : Allow user to set ID
func (_options *RewrapKeyOptions) SetID(id string) *RewrapKeyOptions {
	_options.ID = core.StringPtr(id)
	return _options
}

// SetBluemixInstance : Allow user to set BluemixInstance
func (_options *RewrapKeyOptions) SetBluemixInstance(bluemixInstance string) *RewrapKeyOptions {
	_options.BluemixInstance = core.StringPtr(bluemixInstance)
	return _options
}

// SetKeyActionRewrapBody : Allow user to set KeyActionRewrapBody
func (_options *RewrapKeyOptions) SetKeyActionRewrapBody(keyActionRewrapBody io.ReadCloser) *RewrapKeyOptions {
	_options.KeyActionRewrapBody = keyActionRewrapBody
	return _options
}

// SetCorrelationID : Allow user to set CorrelationID
func (_options *RewrapKeyOptions) SetCorrelationID(correlationID string) *RewrapKeyOptions {
	_options.CorrelationID = core.StringPtr(correlationID)
	return _options
}

// SetXKmsKeyRing : Allow user to set XKmsKeyRing
func (_options *RewrapKeyOptions) SetXKmsKeyRing(xKmsKeyRing string) *RewrapKeyOptions {
	_options.XKmsKeyRing = core.StringPtr(xKmsKeyRing)
	return _options
}

// SetHeaders : Allow user to set Headers
func (options *RewrapKeyOptions) SetHeaders(param map[string]string) *RewrapKeyOptions {
	options.Headers = param
	return options
}

// RewrapKeyResponseBody : Properties that are associated with the response body of an rewrap action.
type RewrapKeyResponseBody struct {
	// The wrapped data encryption key (WDEK) that you can export to your app or service. The ciphertext contains the DEK
	// wrapped by the latest version of the key (WDEK). It is recommended to store and use this WDEK in future calls to Key
	// Protect. The value is base64 encoded.
	Ciphertext *string `json:"ciphertext,omitempty"`

	// The key version that was used to wrap the DEK. This key version is associated with the `ciphertext` value that was
	// used in the request.
	KeyVersion *WrappedKeyVersionKeyVersion `json:"keyVersion,omitempty"`

	// The latest key version that was used to rewrap the DEK. This key version is associated with the `ciphertext` value
	// that's returned in the response.
	RewrappedKeyVersion *RewrappedKeyVersionRewrappedKeyVersion `json:"rewrappedKeyVersion,omitempty"`
}

// UnmarshalRewrapKeyResponseBody unmarshals an instance of RewrapKeyResponseBody from the specified map of raw messages.
func UnmarshalRewrapKeyResponseBody(m map[string]json.RawMessage, result interface{}) (err error) {
	obj := new(RewrapKeyResponseBody)
	err = core.UnmarshalPrimitive(m, "ciphertext", &obj.Ciphertext)
	if err != nil {
		err = core.SDKErrorf(err, "", "ciphertext-error", common.GetComponentInfo())
		return
	}
	err = core.UnmarshalModel(m, "keyVersion", &obj.KeyVersion, UnmarshalWrappedKeyVersionKeyVersion)
	if err != nil {
		err = core.SDKErrorf(err, "", "keyVersion-error", common.GetComponentInfo())
		return
	}
	err = core.UnmarshalModel(m, "rewrappedKeyVersion", &obj.RewrappedKeyVersion, UnmarshalRewrappedKeyVersionRewrappedKeyVersion)
	if err != nil {
		err = core.SDKErrorf(err, "", "rewrappedKeyVersion-error", common.GetComponentInfo())
		return
	}
	reflect.ValueOf(result).Elem().Set(reflect.ValueOf(obj))
	return
}

// RewrappedKeyVersionRewrappedKeyVersion : The latest key version that was used to rewrap the DEK. This key version is associated with the `ciphertext` value
// that's returned in the response.
type RewrappedKeyVersionRewrappedKeyVersion struct {
	// The ID of the key version.
	ID *string `json:"id,omitempty"`
}

// UnmarshalRewrappedKeyVersionRewrappedKeyVersion unmarshals an instance of RewrappedKeyVersionRewrappedKeyVersion from the specified map of raw messages.
func UnmarshalRewrappedKeyVersionRewrappedKeyVersion(m map[string]json.RawMessage, result interface{}) (err error) {
	obj := new(RewrappedKeyVersionRewrappedKeyVersion)
	err = core.UnmarshalPrimitive(m, "id", &obj.ID)
	if err != nil {
		err = core.SDKErrorf(err, "", "id-error", common.GetComponentInfo())
		return
	}
	reflect.ValueOf(result).Elem().Set(reflect.ValueOf(obj))
	return
}

// RotateKeyOptions : The RotateKey options.
type RotateKeyOptions struct {
	// The v4 UUID or alias that uniquely identifies the key.
	ID *string `json:"id" validate:"required,ne="`

	// The IBM Cloud instance ID that identifies your Key Protect service instance.
	BluemixInstance *string `json:"Bluemix-Instance" validate:"required"`

	// The base request for rotate key action.
	KeyActionRotateBody io.ReadCloser `json:"KeyActionRotateBody,omitempty"`

	// The v4 UUID used to correlate and track transactions.
	CorrelationID *string `json:"Correlation-Id,omitempty"`

	// The ID of the key ring that the specified key is a part of. When the header is not specified, Key Protect will
	// perform a key ring lookup. For a more optimized request, specify the key ring on every call. The key ring ID of keys
	// that are created without an `X-Kms-Key-Ring` header is: `default`.
	XKmsKeyRing *string `json:"X-Kms-Key-Ring,omitempty"`

	// Alters server behavior for POST or DELETE operations. A header with `return=minimal` causes the service to return
	// only the key identifier as metadata. A header containing `return=representation` returns both the key material and
	// metadata in the response entity-body. If the key has been designated as a root key, the system cannot return the key
	// material.
	// **Note:** During POST operations, Key Protect may not immediately return the key material due to key generation
	// time. To retrieve the key material, you can perform a subsequent `GET /keys/{id}` request.
	Prefer *string `json:"Prefer,omitempty"`

	// Allows users to set headers on API requests.
	Headers map[string]string
}

// Constants associated with the RotateKeyOptions.Prefer property.
// Alters server behavior for POST or DELETE operations. A header with `return=minimal` causes the service to return
// only the key identifier as metadata. A header containing `return=representation` returns both the key material and
// metadata in the response entity-body. If the key has been designated as a root key, the system cannot return the key
// material.
// **Note:** During POST operations, Key Protect may not immediately return the key material due to key generation time.
// To retrieve the key material, you can perform a subsequent `GET /keys/{id}` request.
const (
	RotateKeyOptions_Prefer_ReturnMinimal        = "return=minimal"
	RotateKeyOptions_Prefer_ReturnRepresentation = "return=representation"
)

// NewRotateKeyOptions : Instantiate RotateKeyOptions
func (*IbmKeyProtectApiV2) NewRotateKeyOptions(id string, bluemixInstance string) *RotateKeyOptions {
	return &RotateKeyOptions{
		ID:              core.StringPtr(id),
		BluemixInstance: core.StringPtr(bluemixInstance),
	}
}

// SetID : Allow user to set ID
func (_options *RotateKeyOptions) SetID(id string) *RotateKeyOptions {
	_options.ID = core.StringPtr(id)
	return _options
}

// SetBluemixInstance : Allow user to set BluemixInstance
func (_options *RotateKeyOptions) SetBluemixInstance(bluemixInstance string) *RotateKeyOptions {
	_options.BluemixInstance = core.StringPtr(bluemixInstance)
	return _options
}

// SetKeyActionRotateBody : Allow user to set KeyActionRotateBody
func (_options *RotateKeyOptions) SetKeyActionRotateBody(keyActionRotateBody io.ReadCloser) *RotateKeyOptions {
	_options.KeyActionRotateBody = keyActionRotateBody
	return _options
}

// SetCorrelationID : Allow user to set CorrelationID
func (_options *RotateKeyOptions) SetCorrelationID(correlationID string) *RotateKeyOptions {
	_options.CorrelationID = core.StringPtr(correlationID)
	return _options
}

// SetXKmsKeyRing : Allow user to set XKmsKeyRing
func (_options *RotateKeyOptions) SetXKmsKeyRing(xKmsKeyRing string) *RotateKeyOptions {
	_options.XKmsKeyRing = core.StringPtr(xKmsKeyRing)
	return _options
}

// SetPrefer : Allow user to set Prefer
func (_options *RotateKeyOptions) SetPrefer(prefer string) *RotateKeyOptions {
	_options.Prefer = core.StringPtr(prefer)
	return _options
}

// SetHeaders : Allow user to set Headers
func (options *RotateKeyOptions) SetHeaders(param map[string]string) *RotateKeyOptions {
	options.Headers = param
	return options
}

// RotationKeyMetadata : Metadata that indicates the status of a rotation policy on the key.
type RotationKeyMetadata struct {
	// If set to `true`, Key Protect enables a rotation policy on a single key.
	Enabled *bool `json:"enabled" validate:"required"`

	// Specifies the key rotation time interval in approximate months, where a month is equivalent to 30 days. A minimum of
	// 1 and a maximum of 12 can be set.
	IntervalMonth *int64 `json:"interval_month,omitempty"`
}

// UnmarshalRotationKeyMetadata unmarshals an instance of RotationKeyMetadata from the specified map of raw messages.
func UnmarshalRotationKeyMetadata(m map[string]json.RawMessage, result interface{}) (err error) {
	obj := new(RotationKeyMetadata)
	err = core.UnmarshalPrimitive(m, "enabled", &obj.Enabled)
	if err != nil {
		err = core.SDKErrorf(err, "", "enabled-error", common.GetComponentInfo())
		return
	}
	err = core.UnmarshalPrimitive(m, "interval_month", &obj.IntervalMonth)
	if err != nil {
		err = core.SDKErrorf(err, "", "interval_month-error", common.GetComponentInfo())
		return
	}
	reflect.ValueOf(result).Elem().Set(reflect.ValueOf(obj))
	return
}

// SetInstancePoliciesOneOf : SetInstancePoliciesOneOf struct
// Models which "extend" this model:
// - SetInstancePoliciesOneOfSetInstancePolicyAllowedNetwork
// - SetInstancePoliciesOneOfSetInstancePolicyDualAuthDelete
// - SetInstancePoliciesOneOfSetInstancePolicyAllowedIP
// - SetInstancePoliciesOneOfSetInstancePolicyKeyCreateImportAccess
// - SetInstancePoliciesOneOfSetInstancePolicyMetrics
// - SetInstancePoliciesOneOfSetInstancePolicyRotation
// - SetInstancePoliciesOneOfSetMultipleInstancePolicies
type SetInstancePoliciesOneOf struct {
	// The metadata that describes the resource array.
	Metadata *CollectionMetadata `json:"metadata,omitempty"`

	// A collection of resources.
	Resources []SetInstancePoliciesOneOfResourcesItem `json:"resources,omitempty"`
}

func (*SetInstancePoliciesOneOf) isaSetInstancePoliciesOneOf() bool {
	return true
}

type SetInstancePoliciesOneOfIntf interface {
	isaSetInstancePoliciesOneOf() bool
}

// UnmarshalSetInstancePoliciesOneOf unmarshals an instance of SetInstancePoliciesOneOf from the specified map of raw messages.
func UnmarshalSetInstancePoliciesOneOf(m map[string]json.RawMessage, result interface{}) (err error) {
	obj := new(SetInstancePoliciesOneOf)
	err = core.UnmarshalModel(m, "metadata", &obj.Metadata, UnmarshalCollectionMetadata)
	if err != nil {
		err = core.SDKErrorf(err, "", "metadata-error", common.GetComponentInfo())
		return
	}
	err = core.UnmarshalModel(m, "resources", &obj.Resources, UnmarshalSetInstancePoliciesOneOfResourcesItem)
	if err != nil {
		err = core.SDKErrorf(err, "", "resources-error", common.GetComponentInfo())
		return
	}
	reflect.ValueOf(result).Elem().Set(reflect.ValueOf(obj))
	return
}

// SetInstancePoliciesOneOfResourcesItem : SetInstancePoliciesOneOfResourcesItem struct
type SetInstancePoliciesOneOfResourcesItem struct {
	// The type of policy to be set.
	PolicyType *string `json:"policy_type" validate:"required"`

	// User defined metadata that is associated with the `allowedNetwork` instance policy type.
	PolicyData *InstancePolicyAllowedNetworkPolicyData `json:"policy_data" validate:"required"`
}

// Constants associated with the SetInstancePoliciesOneOfResourcesItem.PolicyType property.
// The type of policy to be set.
const (
	SetInstancePoliciesOneOfResourcesItem_PolicyType_Allowednetwork = "allowedNetwork"
)

// NewSetInstancePoliciesOneOfResourcesItem : Instantiate SetInstancePoliciesOneOfResourcesItem (Generic Model Constructor)
func (*IbmKeyProtectApiV2) NewSetInstancePoliciesOneOfResourcesItem(policyType string, policyData *InstancePolicyAllowedNetworkPolicyData) (_model *SetInstancePoliciesOneOfResourcesItem, err error) {
	_model = &SetInstancePoliciesOneOfResourcesItem{
		PolicyType: core.StringPtr(policyType),
		PolicyData: policyData,
	}
	err = core.ValidateStruct(_model, "required parameters")
	if err != nil {
		err = core.SDKErrorf(err, "", "model-missing-required", common.GetComponentInfo())
	}
	return
}

// UnmarshalSetInstancePoliciesOneOfResourcesItem unmarshals an instance of SetInstancePoliciesOneOfResourcesItem from the specified map of raw messages.
func UnmarshalSetInstancePoliciesOneOfResourcesItem(m map[string]json.RawMessage, result interface{}) (err error) {
	obj := new(SetInstancePoliciesOneOfResourcesItem)
	err = core.UnmarshalPrimitive(m, "policy_type", &obj.PolicyType)
	if err != nil {
		err = core.SDKErrorf(err, "", "policy_type-error", common.GetComponentInfo())
		return
	}
	err = core.UnmarshalModel(m, "policy_data", &obj.PolicyData, UnmarshalInstancePolicyAllowedNetworkPolicyData)
	if err != nil {
		err = core.SDKErrorf(err, "", "policy_data-error", common.GetComponentInfo())
		return
	}
	reflect.ValueOf(result).Elem().Set(reflect.ValueOf(obj))
	return
}

// SetInstancePoliciesOneOfSetInstancePolicyAllowedIPResourcesItem : SetInstancePoliciesOneOfSetInstancePolicyAllowedIPResourcesItem struct
type SetInstancePoliciesOneOfSetInstancePolicyAllowedIPResourcesItem struct {
	// The type of policy to be set.
	PolicyType *string `json:"policy_type" validate:"required"`

	// User defined metadata that is associated with the `allowedIP` instance policy type.
	PolicyData *InstancePolicyAllowedIPPolicyData `json:"policy_data" validate:"required"`
}

// Constants associated with the SetInstancePoliciesOneOfSetInstancePolicyAllowedIPResourcesItem.PolicyType property.
// The type of policy to be set.
const (
	SetInstancePoliciesOneOfSetInstancePolicyAllowedIPResourcesItem_PolicyType_Allowedip = "allowedIP"
)

// NewSetInstancePoliciesOneOfSetInstancePolicyAllowedIPResourcesItem : Instantiate SetInstancePoliciesOneOfSetInstancePolicyAllowedIPResourcesItem (Generic Model Constructor)
func (*IbmKeyProtectApiV2) NewSetInstancePoliciesOneOfSetInstancePolicyAllowedIPResourcesItem(policyType string, policyData *InstancePolicyAllowedIPPolicyData) (_model *SetInstancePoliciesOneOfSetInstancePolicyAllowedIPResourcesItem, err error) {
	_model = &SetInstancePoliciesOneOfSetInstancePolicyAllowedIPResourcesItem{
		PolicyType: core.StringPtr(policyType),
		PolicyData: policyData,
	}
	err = core.ValidateStruct(_model, "required parameters")
	if err != nil {
		err = core.SDKErrorf(err, "", "model-missing-required", common.GetComponentInfo())
	}
	return
}

// UnmarshalSetInstancePoliciesOneOfSetInstancePolicyAllowedIPResourcesItem unmarshals an instance of SetInstancePoliciesOneOfSetInstancePolicyAllowedIPResourcesItem from the specified map of raw messages.
func UnmarshalSetInstancePoliciesOneOfSetInstancePolicyAllowedIPResourcesItem(m map[string]json.RawMessage, result interface{}) (err error) {
	obj := new(SetInstancePoliciesOneOfSetInstancePolicyAllowedIPResourcesItem)
	err = core.UnmarshalPrimitive(m, "policy_type", &obj.PolicyType)
	if err != nil {
		err = core.SDKErrorf(err, "", "policy_type-error", common.GetComponentInfo())
		return
	}
	err = core.UnmarshalModel(m, "policy_data", &obj.PolicyData, UnmarshalInstancePolicyAllowedIPPolicyData)
	if err != nil {
		err = core.SDKErrorf(err, "", "policy_data-error", common.GetComponentInfo())
		return
	}
	reflect.ValueOf(result).Elem().Set(reflect.ValueOf(obj))
	return
}

// SetInstancePoliciesOneOfSetInstancePolicyAllowedNetworkResourcesItem : SetInstancePoliciesOneOfSetInstancePolicyAllowedNetworkResourcesItem struct
type SetInstancePoliciesOneOfSetInstancePolicyAllowedNetworkResourcesItem struct {
	// The type of policy to be set.
	PolicyType *string `json:"policy_type" validate:"required"`

	// User defined metadata that is associated with the `allowedNetwork` instance policy type.
	PolicyData *InstancePolicyAllowedNetworkPolicyData `json:"policy_data" validate:"required"`
}

// Constants associated with the SetInstancePoliciesOneOfSetInstancePolicyAllowedNetworkResourcesItem.PolicyType property.
// The type of policy to be set.
const (
	SetInstancePoliciesOneOfSetInstancePolicyAllowedNetworkResourcesItem_PolicyType_Allowednetwork = "allowedNetwork"
)

// NewSetInstancePoliciesOneOfSetInstancePolicyAllowedNetworkResourcesItem : Instantiate SetInstancePoliciesOneOfSetInstancePolicyAllowedNetworkResourcesItem (Generic Model Constructor)
func (*IbmKeyProtectApiV2) NewSetInstancePoliciesOneOfSetInstancePolicyAllowedNetworkResourcesItem(policyType string, policyData *InstancePolicyAllowedNetworkPolicyData) (_model *SetInstancePoliciesOneOfSetInstancePolicyAllowedNetworkResourcesItem, err error) {
	_model = &SetInstancePoliciesOneOfSetInstancePolicyAllowedNetworkResourcesItem{
		PolicyType: core.StringPtr(policyType),
		PolicyData: policyData,
	}
	err = core.ValidateStruct(_model, "required parameters")
	if err != nil {
		err = core.SDKErrorf(err, "", "model-missing-required", common.GetComponentInfo())
	}
	return
}

// UnmarshalSetInstancePoliciesOneOfSetInstancePolicyAllowedNetworkResourcesItem unmarshals an instance of SetInstancePoliciesOneOfSetInstancePolicyAllowedNetworkResourcesItem from the specified map of raw messages.
func UnmarshalSetInstancePoliciesOneOfSetInstancePolicyAllowedNetworkResourcesItem(m map[string]json.RawMessage, result interface{}) (err error) {
	obj := new(SetInstancePoliciesOneOfSetInstancePolicyAllowedNetworkResourcesItem)
	err = core.UnmarshalPrimitive(m, "policy_type", &obj.PolicyType)
	if err != nil {
		err = core.SDKErrorf(err, "", "policy_type-error", common.GetComponentInfo())
		return
	}
	err = core.UnmarshalModel(m, "policy_data", &obj.PolicyData, UnmarshalInstancePolicyAllowedNetworkPolicyData)
	if err != nil {
		err = core.SDKErrorf(err, "", "policy_data-error", common.GetComponentInfo())
		return
	}
	reflect.ValueOf(result).Elem().Set(reflect.ValueOf(obj))
	return
}

// SetInstancePoliciesOneOfSetInstancePolicyKeyCreateImportAccessResourcesItem : SetInstancePoliciesOneOfSetInstancePolicyKeyCreateImportAccessResourcesItem struct
type SetInstancePoliciesOneOfSetInstancePolicyKeyCreateImportAccessResourcesItem struct {
	// The type of policy to be set.
	PolicyType *string `json:"policy_type" validate:"required"`

	// User defined metadata that is associated with the `keyCreateImportAccess` instance policy type.
	PolicyData *InstancePolicyKeyCreateImportAccessPolicyData `json:"policy_data" validate:"required"`
}

// Constants associated with the SetInstancePoliciesOneOfSetInstancePolicyKeyCreateImportAccessResourcesItem.PolicyType property.
// The type of policy to be set.
const (
	SetInstancePoliciesOneOfSetInstancePolicyKeyCreateImportAccessResourcesItem_PolicyType_Keycreateimportaccess = "keyCreateImportAccess"
)

// NewSetInstancePoliciesOneOfSetInstancePolicyKeyCreateImportAccessResourcesItem : Instantiate SetInstancePoliciesOneOfSetInstancePolicyKeyCreateImportAccessResourcesItem (Generic Model Constructor)
func (*IbmKeyProtectApiV2) NewSetInstancePoliciesOneOfSetInstancePolicyKeyCreateImportAccessResourcesItem(policyType string, policyData *InstancePolicyKeyCreateImportAccessPolicyData) (_model *SetInstancePoliciesOneOfSetInstancePolicyKeyCreateImportAccessResourcesItem, err error) {
	_model = &SetInstancePoliciesOneOfSetInstancePolicyKeyCreateImportAccessResourcesItem{
		PolicyType: core.StringPtr(policyType),
		PolicyData: policyData,
	}
	err = core.ValidateStruct(_model, "required parameters")
	if err != nil {
		err = core.SDKErrorf(err, "", "model-missing-required", common.GetComponentInfo())
	}
	return
}

// UnmarshalSetInstancePoliciesOneOfSetInstancePolicyKeyCreateImportAccessResourcesItem unmarshals an instance of SetInstancePoliciesOneOfSetInstancePolicyKeyCreateImportAccessResourcesItem from the specified map of raw messages.
func UnmarshalSetInstancePoliciesOneOfSetInstancePolicyKeyCreateImportAccessResourcesItem(m map[string]json.RawMessage, result interface{}) (err error) {
	obj := new(SetInstancePoliciesOneOfSetInstancePolicyKeyCreateImportAccessResourcesItem)
	err = core.UnmarshalPrimitive(m, "policy_type", &obj.PolicyType)
	if err != nil {
		err = core.SDKErrorf(err, "", "policy_type-error", common.GetComponentInfo())
		return
	}
	err = core.UnmarshalModel(m, "policy_data", &obj.PolicyData, UnmarshalInstancePolicyKeyCreateImportAccessPolicyData)
	if err != nil {
		err = core.SDKErrorf(err, "", "policy_data-error", common.GetComponentInfo())
		return
	}
	reflect.ValueOf(result).Elem().Set(reflect.ValueOf(obj))
	return
}

// SetInstancePoliciesOneOfSetInstancePolicyMetricsResourcesItem : SetInstancePoliciesOneOfSetInstancePolicyMetricsResourcesItem struct
type SetInstancePoliciesOneOfSetInstancePolicyMetricsResourcesItem struct {
	// The type of policy to be set.
	PolicyType *string `json:"policy_type" validate:"required"`

	// User defined metadata that is associated with the `metrics` instance policy type.
	PolicyData *MetricsProperties `json:"policy_data" validate:"required"`
}

// Constants associated with the SetInstancePoliciesOneOfSetInstancePolicyMetricsResourcesItem.PolicyType property.
// The type of policy to be set.
const (
	SetInstancePoliciesOneOfSetInstancePolicyMetricsResourcesItem_PolicyType_Metrics = "metrics"
)

// NewSetInstancePoliciesOneOfSetInstancePolicyMetricsResourcesItem : Instantiate SetInstancePoliciesOneOfSetInstancePolicyMetricsResourcesItem (Generic Model Constructor)
func (*IbmKeyProtectApiV2) NewSetInstancePoliciesOneOfSetInstancePolicyMetricsResourcesItem(policyType string, policyData *MetricsProperties) (_model *SetInstancePoliciesOneOfSetInstancePolicyMetricsResourcesItem, err error) {
	_model = &SetInstancePoliciesOneOfSetInstancePolicyMetricsResourcesItem{
		PolicyType: core.StringPtr(policyType),
		PolicyData: policyData,
	}
	err = core.ValidateStruct(_model, "required parameters")
	if err != nil {
		err = core.SDKErrorf(err, "", "model-missing-required", common.GetComponentInfo())
	}
	return
}

// UnmarshalSetInstancePoliciesOneOfSetInstancePolicyMetricsResourcesItem unmarshals an instance of SetInstancePoliciesOneOfSetInstancePolicyMetricsResourcesItem from the specified map of raw messages.
func UnmarshalSetInstancePoliciesOneOfSetInstancePolicyMetricsResourcesItem(m map[string]json.RawMessage, result interface{}) (err error) {
	obj := new(SetInstancePoliciesOneOfSetInstancePolicyMetricsResourcesItem)
	err = core.UnmarshalPrimitive(m, "policy_type", &obj.PolicyType)
	if err != nil {
		err = core.SDKErrorf(err, "", "policy_type-error", common.GetComponentInfo())
		return
	}
	err = core.UnmarshalModel(m, "policy_data", &obj.PolicyData, UnmarshalMetricsProperties)
	if err != nil {
		err = core.SDKErrorf(err, "", "policy_data-error", common.GetComponentInfo())
		return
	}
	reflect.ValueOf(result).Elem().Set(reflect.ValueOf(obj))
	return
}

// SetInstancePoliciesOneOfSetInstancePolicyRotationResourcesItem : SetInstancePoliciesOneOfSetInstancePolicyRotationResourcesItem struct
type SetInstancePoliciesOneOfSetInstancePolicyRotationResourcesItem struct {
	// The type of policy to be set.
	PolicyType *string `json:"policy_type" validate:"required"`

	// User defined metadata that is associated with the `rotation` instance policy type.
	PolicyData *InstancePolicyRotationPolicyData `json:"policy_data" validate:"required"`
}

// Constants associated with the SetInstancePoliciesOneOfSetInstancePolicyRotationResourcesItem.PolicyType property.
// The type of policy to be set.
const (
	SetInstancePoliciesOneOfSetInstancePolicyRotationResourcesItem_PolicyType_Rotation = "rotation"
)

// NewSetInstancePoliciesOneOfSetInstancePolicyRotationResourcesItem : Instantiate SetInstancePoliciesOneOfSetInstancePolicyRotationResourcesItem (Generic Model Constructor)
func (*IbmKeyProtectApiV2) NewSetInstancePoliciesOneOfSetInstancePolicyRotationResourcesItem(policyType string, policyData *InstancePolicyRotationPolicyData) (_model *SetInstancePoliciesOneOfSetInstancePolicyRotationResourcesItem, err error) {
	_model = &SetInstancePoliciesOneOfSetInstancePolicyRotationResourcesItem{
		PolicyType: core.StringPtr(policyType),
		PolicyData: policyData,
	}
	err = core.ValidateStruct(_model, "required parameters")
	if err != nil {
		err = core.SDKErrorf(err, "", "model-missing-required", common.GetComponentInfo())
	}
	return
}

// UnmarshalSetInstancePoliciesOneOfSetInstancePolicyRotationResourcesItem unmarshals an instance of SetInstancePoliciesOneOfSetInstancePolicyRotationResourcesItem from the specified map of raw messages.
func UnmarshalSetInstancePoliciesOneOfSetInstancePolicyRotationResourcesItem(m map[string]json.RawMessage, result interface{}) (err error) {
	obj := new(SetInstancePoliciesOneOfSetInstancePolicyRotationResourcesItem)
	err = core.UnmarshalPrimitive(m, "policy_type", &obj.PolicyType)
	if err != nil {
		err = core.SDKErrorf(err, "", "policy_type-error", common.GetComponentInfo())
		return
	}
	err = core.UnmarshalModel(m, "policy_data", &obj.PolicyData, UnmarshalInstancePolicyRotationPolicyData)
	if err != nil {
		err = core.SDKErrorf(err, "", "policy_data-error", common.GetComponentInfo())
		return
	}
	reflect.ValueOf(result).Elem().Set(reflect.ValueOf(obj))
	return
}

// SetInstancePolicyDualAuthDeleteResourcesItem : SetInstancePolicyDualAuthDeleteResourcesItem struct
type SetInstancePolicyDualAuthDeleteResourcesItem struct {
	// The type of policy to be set.
	PolicyType *string `json:"policy_type" validate:"required"`

	// User defined metadata that is associated with the `dualAuthDelete` instance policy type.
	PolicyData *DualAuthDeleteProperties `json:"policy_data" validate:"required"`
}

// Constants associated with the SetInstancePolicyDualAuthDeleteResourcesItem.PolicyType property.
// The type of policy to be set.
const (
	SetInstancePolicyDualAuthDeleteResourcesItem_PolicyType_Dualauthdelete = "dualAuthDelete"
)

// NewSetInstancePolicyDualAuthDeleteResourcesItem : Instantiate SetInstancePolicyDualAuthDeleteResourcesItem (Generic Model Constructor)
func (*IbmKeyProtectApiV2) NewSetInstancePolicyDualAuthDeleteResourcesItem(policyType string, policyData *DualAuthDeleteProperties) (_model *SetInstancePolicyDualAuthDeleteResourcesItem, err error) {
	_model = &SetInstancePolicyDualAuthDeleteResourcesItem{
		PolicyType: core.StringPtr(policyType),
		PolicyData: policyData,
	}
	err = core.ValidateStruct(_model, "required parameters")
	if err != nil {
		err = core.SDKErrorf(err, "", "model-missing-required", common.GetComponentInfo())
	}
	return
}

// UnmarshalSetInstancePolicyDualAuthDeleteResourcesItem unmarshals an instance of SetInstancePolicyDualAuthDeleteResourcesItem from the specified map of raw messages.
func UnmarshalSetInstancePolicyDualAuthDeleteResourcesItem(m map[string]json.RawMessage, result interface{}) (err error) {
	obj := new(SetInstancePolicyDualAuthDeleteResourcesItem)
	err = core.UnmarshalPrimitive(m, "policy_type", &obj.PolicyType)
	if err != nil {
		err = core.SDKErrorf(err, "", "policy_type-error", common.GetComponentInfo())
		return
	}
	err = core.UnmarshalModel(m, "policy_data", &obj.PolicyData, UnmarshalDualAuthDeleteProperties)
	if err != nil {
		err = core.SDKErrorf(err, "", "policy_data-error", common.GetComponentInfo())
		return
	}
	reflect.ValueOf(result).Elem().Set(reflect.ValueOf(obj))
	return
}

// SetKeyForDeletionOptions : The SetKeyForDeletion options.
type SetKeyForDeletionOptions struct {
	// The v4 UUID or alias that uniquely identifies the key.
	ID *string `json:"id" validate:"required,ne="`

	// The IBM Cloud instance ID that identifies your Key Protect service instance.
	BluemixInstance *string `json:"Bluemix-Instance" validate:"required"`

	// The v4 UUID used to correlate and track transactions.
	CorrelationID *string `json:"Correlation-Id,omitempty"`

	// The ID of the key ring that the specified key is a part of. When the header is not specified, Key Protect will
	// perform a key ring lookup. For a more optimized request, specify the key ring on every call. The key ring ID of keys
	// that are created without an `X-Kms-Key-Ring` header is: `default`.
	XKmsKeyRing *string `json:"X-Kms-Key-Ring,omitempty"`

	// Allows users to set headers on API requests.
	Headers map[string]string
}

// NewSetKeyForDeletionOptions : Instantiate SetKeyForDeletionOptions
func (*IbmKeyProtectApiV2) NewSetKeyForDeletionOptions(id string, bluemixInstance string) *SetKeyForDeletionOptions {
	return &SetKeyForDeletionOptions{
		ID:              core.StringPtr(id),
		BluemixInstance: core.StringPtr(bluemixInstance),
	}
}

// SetID : Allow user to set ID
func (_options *SetKeyForDeletionOptions) SetID(id string) *SetKeyForDeletionOptions {
	_options.ID = core.StringPtr(id)
	return _options
}

// SetBluemixInstance : Allow user to set BluemixInstance
func (_options *SetKeyForDeletionOptions) SetBluemixInstance(bluemixInstance string) *SetKeyForDeletionOptions {
	_options.BluemixInstance = core.StringPtr(bluemixInstance)
	return _options
}

// SetCorrelationID : Allow user to set CorrelationID
func (_options *SetKeyForDeletionOptions) SetCorrelationID(correlationID string) *SetKeyForDeletionOptions {
	_options.CorrelationID = core.StringPtr(correlationID)
	return _options
}

// SetXKmsKeyRing : Allow user to set XKmsKeyRing
func (_options *SetKeyForDeletionOptions) SetXKmsKeyRing(xKmsKeyRing string) *SetKeyForDeletionOptions {
	_options.XKmsKeyRing = core.StringPtr(xKmsKeyRing)
	return _options
}

// SetHeaders : Allow user to set Headers
func (options *SetKeyForDeletionOptions) SetHeaders(param map[string]string) *SetKeyForDeletionOptions {
	options.Headers = param
	return options
}

// SetKeyPoliciesOneOf : SetKeyPoliciesOneOf struct
// Models which "extend" this model:
// - SetKeyPoliciesOneOfSetKeyPolicyDualAuthDelete
// - SetKeyPoliciesOneOfSetKeyPolicyRotation
// - SetKeyPoliciesOneOfSetMultipleKeyPolicies
type SetKeyPoliciesOneOf struct {
	// The metadata that describes the resource array.
	Metadata *CollectionMetadata `json:"metadata,omitempty"`

	// A collection of resources.
	Resources []KeyPolicyDualAuthDelete `json:"resources,omitempty"`
}

func (*SetKeyPoliciesOneOf) isaSetKeyPoliciesOneOf() bool {
	return true
}

type SetKeyPoliciesOneOfIntf interface {
	isaSetKeyPoliciesOneOf() bool
}

// UnmarshalSetKeyPoliciesOneOf unmarshals an instance of SetKeyPoliciesOneOf from the specified map of raw messages.
func UnmarshalSetKeyPoliciesOneOf(m map[string]json.RawMessage, result interface{}) (err error) {
	obj := new(SetKeyPoliciesOneOf)
	err = core.UnmarshalModel(m, "metadata", &obj.Metadata, UnmarshalCollectionMetadata)
	if err != nil {
		err = core.SDKErrorf(err, "", "metadata-error", common.GetComponentInfo())
		return
	}
	err = core.UnmarshalModel(m, "resources", &obj.Resources, UnmarshalKeyPolicyDualAuthDelete)
	if err != nil {
		err = core.SDKErrorf(err, "", "resources-error", common.GetComponentInfo())
		return
	}
	reflect.ValueOf(result).Elem().Set(reflect.ValueOf(obj))
	return
}

// SetMultipleInstancePoliciesResourcesItem : SetMultipleInstancePoliciesResourcesItem struct
type SetMultipleInstancePoliciesResourcesItem struct {
	// The type of policy to be set.
	PolicyType *string `json:"policy_type" validate:"required"`

	// User defined metadata that is associated with any instance policy.
	PolicyData *SetMultipleInstancePoliciesResourcesItemPolicyData `json:"policy_data" validate:"required"`
}

// Constants associated with the SetMultipleInstancePoliciesResourcesItem.PolicyType property.
// The type of policy to be set.
const (
	SetMultipleInstancePoliciesResourcesItem_PolicyType_Allowedip             = "allowedIP"
	SetMultipleInstancePoliciesResourcesItem_PolicyType_Allowednetwork        = "allowedNetwork"
	SetMultipleInstancePoliciesResourcesItem_PolicyType_Dualauthdelete        = "dualAuthDelete"
	SetMultipleInstancePoliciesResourcesItem_PolicyType_Keycreateimportaccess = "keyCreateImportAccess"
	SetMultipleInstancePoliciesResourcesItem_PolicyType_Metrics               = "metrics"
	SetMultipleInstancePoliciesResourcesItem_PolicyType_Rotation              = "rotation"
)

// NewSetMultipleInstancePoliciesResourcesItem : Instantiate SetMultipleInstancePoliciesResourcesItem (Generic Model Constructor)
func (*IbmKeyProtectApiV2) NewSetMultipleInstancePoliciesResourcesItem(policyType string, policyData *SetMultipleInstancePoliciesResourcesItemPolicyData) (_model *SetMultipleInstancePoliciesResourcesItem, err error) {
	_model = &SetMultipleInstancePoliciesResourcesItem{
		PolicyType: core.StringPtr(policyType),
		PolicyData: policyData,
	}
	err = core.ValidateStruct(_model, "required parameters")
	if err != nil {
		err = core.SDKErrorf(err, "", "model-missing-required", common.GetComponentInfo())
	}
	return
}

// UnmarshalSetMultipleInstancePoliciesResourcesItem unmarshals an instance of SetMultipleInstancePoliciesResourcesItem from the specified map of raw messages.
func UnmarshalSetMultipleInstancePoliciesResourcesItem(m map[string]json.RawMessage, result interface{}) (err error) {
	obj := new(SetMultipleInstancePoliciesResourcesItem)
	err = core.UnmarshalPrimitive(m, "policy_type", &obj.PolicyType)
	if err != nil {
		err = core.SDKErrorf(err, "", "policy_type-error", common.GetComponentInfo())
		return
	}
	err = core.UnmarshalModel(m, "policy_data", &obj.PolicyData, UnmarshalSetMultipleInstancePoliciesResourcesItemPolicyData)
	if err != nil {
		err = core.SDKErrorf(err, "", "policy_data-error", common.GetComponentInfo())
		return
	}
	reflect.ValueOf(result).Elem().Set(reflect.ValueOf(obj))
	return
}

// SetMultipleInstancePoliciesResourcesItemPolicyData : User defined metadata that is associated with any instance policy.
type SetMultipleInstancePoliciesResourcesItemPolicyData struct {
	// If set to `true`, Key Protect enables the specified policy for your service instance. If set to `false`, Key Protect
	// disables the specified policy for your service instance, and the policy will no longer affect Key Protect actions.
	// **Note:** If a policy with attributes is disabled, all attributes are reset and are not retained.
	Enabled *bool `json:"enabled" validate:"required"`

	// Attributes associated with any instance policy type. Must be provided if the `enabled` field is `true`. Cannot be
	// provided if the `enabled` field is `false`. Only attributes corresponding to the `policy_type` can be provided.
	Attributes *SetMultipleInstancePoliciesResourcesItemPolicyDataAttributes `json:"attributes,omitempty"`
}

// NewSetMultipleInstancePoliciesResourcesItemPolicyData : Instantiate SetMultipleInstancePoliciesResourcesItemPolicyData (Generic Model Constructor)
func (*IbmKeyProtectApiV2) NewSetMultipleInstancePoliciesResourcesItemPolicyData(enabled bool) (_model *SetMultipleInstancePoliciesResourcesItemPolicyData, err error) {
	_model = &SetMultipleInstancePoliciesResourcesItemPolicyData{
		Enabled: core.BoolPtr(enabled),
	}
	err = core.ValidateStruct(_model, "required parameters")
	if err != nil {
		err = core.SDKErrorf(err, "", "model-missing-required", common.GetComponentInfo())
	}
	return
}

// UnmarshalSetMultipleInstancePoliciesResourcesItemPolicyData unmarshals an instance of SetMultipleInstancePoliciesResourcesItemPolicyData from the specified map of raw messages.
func UnmarshalSetMultipleInstancePoliciesResourcesItemPolicyData(m map[string]json.RawMessage, result interface{}) (err error) {
	obj := new(SetMultipleInstancePoliciesResourcesItemPolicyData)
	err = core.UnmarshalPrimitive(m, "enabled", &obj.Enabled)
	if err != nil {
		err = core.SDKErrorf(err, "", "enabled-error", common.GetComponentInfo())
		return
	}
	err = core.UnmarshalModel(m, "attributes", &obj.Attributes, UnmarshalSetMultipleInstancePoliciesResourcesItemPolicyDataAttributes)
	if err != nil {
		err = core.SDKErrorf(err, "", "attributes-error", common.GetComponentInfo())
		return
	}
	reflect.ValueOf(result).Elem().Set(reflect.ValueOf(obj))
	return
}

// SetMultipleInstancePoliciesResourcesItemPolicyDataAttributes : Attributes associated with any instance policy type. Must be provided if the `enabled` field is `true`. Cannot be
// provided if the `enabled` field is `false`. Only attributes corresponding to the `policy_type` can be provided.
type SetMultipleInstancePoliciesResourcesItemPolicyDataAttributes struct {
	// If set to `public-and-private`, Key Protect allows the instance to be accessible through public and private
	// endpoints. If set to `private-only`, Key Protect restricts the instance to only be accessible through a private
	// endpoint.
	AllowedNetwork *string `json:"allowed_network,omitempty"`

	// A string array of IPv4 or IPv6 CIDR notated subnets that are authorized to interact with the instance. If both
	// `allowedNetwork` and `allowedIP` policies are set, only traffic aligning with both the `allowed_network` allowed
	// network policy attribute and the `allowed_ip` allowed IP policy attribute will be allowed. IPv4 and iIP6 addresses
	// are accepted for public endpoints. Only the IPv4 private network gateway addresses from the array will be authorized
	// to access your instance via private endpoint.
	// **Important:** Once set, accessing your instance may require additional steps. For more information, see [Accessing
	// an instance via public
	// endpoint](/docs/key-protect?topic=key-protect-manage-allowed-ip#access-allowed-ip-public-endpoint) and [Accessing an
	// instance via private
	// endpoint](/docs/key-protect?topic=key-protect-manage-allowed-ip#access-allowed-ip-private-endpoint) for more
	// details.
	// **Note:** An allowed IP policy does not affect requests from other IBM Cloud services.
	AllowedIp []string `json:"allowed_ip,omitempty"`

	// If set to `false`, the service prevents you or any authorized users from using Key Protect to create root keys in
	// the specified service instance. If set to `true`, Key Protect allows you or any authorized users to create root keys
	// in the instance.
	// **Note:** If omitted, `POST /instance/policies` will set this attribute to the default value (`true`).
	CreateRootKey *bool `json:"create_root_key,omitempty"`

	// If set to `false`, the service prevents you or any authorized users from using Key Protect to create standard keys
	// in the specified service instance. If set to `true`, Key Protect allows you or any authorized users to create
	// standard keys in the instance.
	// **Note:** If omitted, `POST /instance/policies` will set this attribute to the default value (`true`).
	CreateStandardKey *bool `json:"create_standard_key,omitempty"`

	// If set to `false`, the service prevents you or any authorized users from importing root keys into the specified
	// service instance. If set to `true`, Key Protect allows you or any authorized users to import root keys into the
	// instance.
	// **Note:** If omitted, `POST /instance/policies` will set this attribute to the default value (`true`).
	ImportRootKey *bool `json:"import_root_key,omitempty"`

	// If set to `false`, the service prevents you or any authorized users from importing standard keys into the specified
	// service instance. If set to `true`, Key Protect allows you or any authorized users to import standard keys into the
	// instance.
	// **Note:** If omitted, `POST /instance/policies` will set this attribute to the default value (`true`).
	ImportStandardKey *bool `json:"import_standard_key,omitempty"`

	// If set to `true`, the service prevents you or any authorized users from importing key material into the specified
	// service instance without using an import token. If set to `false`, Key Protect allows you or any authorized users to
	// import key material into the instance without the use of an import token.
	// **Note:** If omitted, `POST /instance/policies` will set this attribute to the default value (`false`).
	EnforceToken *bool `json:"enforce_token,omitempty"`

	// Specifies the key rotation time interval in approximate months, where a month is equivalent to 30 days. A minimum of
	// 1 and a maximum of 12 can be set.
	IntervalMonth *int64 `json:"interval_month,omitempty"`
}

// Constants associated with the SetMultipleInstancePoliciesResourcesItemPolicyDataAttributes.AllowedNetwork property.
// If set to `public-and-private`, Key Protect allows the instance to be accessible through public and private
// endpoints. If set to `private-only`, Key Protect restricts the instance to only be accessible through a private
// endpoint.
const (
	SetMultipleInstancePoliciesResourcesItemPolicyDataAttributes_AllowedNetwork_PrivateOnly      = "private-only"
	SetMultipleInstancePoliciesResourcesItemPolicyDataAttributes_AllowedNetwork_PublicAndPrivate = "public-and-private"
)

// UnmarshalSetMultipleInstancePoliciesResourcesItemPolicyDataAttributes unmarshals an instance of SetMultipleInstancePoliciesResourcesItemPolicyDataAttributes from the specified map of raw messages.
func UnmarshalSetMultipleInstancePoliciesResourcesItemPolicyDataAttributes(m map[string]json.RawMessage, result interface{}) (err error) {
	obj := new(SetMultipleInstancePoliciesResourcesItemPolicyDataAttributes)
	err = core.UnmarshalPrimitive(m, "allowed_network", &obj.AllowedNetwork)
	if err != nil {
		err = core.SDKErrorf(err, "", "allowed_network-error", common.GetComponentInfo())
		return
	}
	err = core.UnmarshalPrimitive(m, "allowed_ip", &obj.AllowedIp)
	if err != nil {
		err = core.SDKErrorf(err, "", "allowed_ip-error", common.GetComponentInfo())
		return
	}
	err = core.UnmarshalPrimitive(m, "create_root_key", &obj.CreateRootKey)
	if err != nil {
		err = core.SDKErrorf(err, "", "create_root_key-error", common.GetComponentInfo())
		return
	}
	err = core.UnmarshalPrimitive(m, "create_standard_key", &obj.CreateStandardKey)
	if err != nil {
		err = core.SDKErrorf(err, "", "create_standard_key-error", common.GetComponentInfo())
		return
	}
	err = core.UnmarshalPrimitive(m, "import_root_key", &obj.ImportRootKey)
	if err != nil {
		err = core.SDKErrorf(err, "", "import_root_key-error", common.GetComponentInfo())
		return
	}
	err = core.UnmarshalPrimitive(m, "import_standard_key", &obj.ImportStandardKey)
	if err != nil {
		err = core.SDKErrorf(err, "", "import_standard_key-error", common.GetComponentInfo())
		return
	}
	err = core.UnmarshalPrimitive(m, "enforce_token", &obj.EnforceToken)
	if err != nil {
		err = core.SDKErrorf(err, "", "enforce_token-error", common.GetComponentInfo())
		return
	}
	err = core.UnmarshalPrimitive(m, "interval_month", &obj.IntervalMonth)
	if err != nil {
		err = core.SDKErrorf(err, "", "interval_month-error", common.GetComponentInfo())
		return
	}
	reflect.ValueOf(result).Elem().Set(reflect.ValueOf(obj))
	return
}

// SetMultipleKeyPoliciesResource : Properties that are associated with key level dual authorization delete policy.
type SetMultipleKeyPoliciesResource struct {
	// Specifies the MIME type that represents the policy resource. Currently, only the default is supported.
	Type *string `json:"type" validate:"required"`

	// Data associated with the dual authorization delete policy.
	DualAuthDelete *KeyPolicyDualAuthDeleteDualAuthDelete `json:"dualAuthDelete" validate:"required"`

	// Data associated with the automatic key rotation policy.
	Rotation *KeyPolicyRotationRotation `json:"rotation" validate:"required"`
}

// Constants associated with the SetMultipleKeyPoliciesResource.Type property.
// Specifies the MIME type that represents the policy resource. Currently, only the default is supported.
const (
	SetMultipleKeyPoliciesResource_Type_ApplicationVndIbmKmsPolicyJSON = "application/vnd.ibm.kms.policy+json"
)

// NewSetMultipleKeyPoliciesResource : Instantiate SetMultipleKeyPoliciesResource (Generic Model Constructor)
func (*IbmKeyProtectApiV2) NewSetMultipleKeyPoliciesResource(typeVar string, dualAuthDelete *KeyPolicyDualAuthDeleteDualAuthDelete, rotation *KeyPolicyRotationRotation) (_model *SetMultipleKeyPoliciesResource, err error) {
	_model = &SetMultipleKeyPoliciesResource{
		Type:           core.StringPtr(typeVar),
		DualAuthDelete: dualAuthDelete,
		Rotation:       rotation,
	}
	err = core.ValidateStruct(_model, "required parameters")
	if err != nil {
		err = core.SDKErrorf(err, "", "model-missing-required", common.GetComponentInfo())
	}
	return
}

// UnmarshalSetMultipleKeyPoliciesResource unmarshals an instance of SetMultipleKeyPoliciesResource from the specified map of raw messages.
func UnmarshalSetMultipleKeyPoliciesResource(m map[string]json.RawMessage, result interface{}) (err error) {
	obj := new(SetMultipleKeyPoliciesResource)
	err = core.UnmarshalPrimitive(m, "type", &obj.Type)
	if err != nil {
		err = core.SDKErrorf(err, "", "type-error", common.GetComponentInfo())
		return
	}
	err = core.UnmarshalModel(m, "dualAuthDelete", &obj.DualAuthDelete, UnmarshalKeyPolicyDualAuthDeleteDualAuthDelete)
	if err != nil {
		err = core.SDKErrorf(err, "", "dualAuthDelete-error", common.GetComponentInfo())
		return
	}
	err = core.UnmarshalModel(m, "rotation", &obj.Rotation, UnmarshalKeyPolicyRotationRotation)
	if err != nil {
		err = core.SDKErrorf(err, "", "rotation-error", common.GetComponentInfo())
		return
	}
	reflect.ValueOf(result).Elem().Set(reflect.ValueOf(obj))
	return
}

// SyncAssociatedResourcesOptions : The SyncAssociatedResources options.
type SyncAssociatedResourcesOptions struct {
	// The v4 UUID or alias that uniquely identifies the key.
	ID *string `json:"id" validate:"required,ne="`

	// The IBM Cloud instance ID that identifies your Key Protect service instance.
	BluemixInstance *string `json:"Bluemix-Instance" validate:"required"`

	// The v4 UUID used to correlate and track transactions.
	CorrelationID *string `json:"Correlation-Id,omitempty"`

	// The ID of the key ring that the specified key is a part of. When the header is not specified, Key Protect will
	// perform a key ring lookup. For a more optimized request, specify the key ring on every call. The key ring ID of keys
	// that are created without an `X-Kms-Key-Ring` header is: `default`.
	XKmsKeyRing *string `json:"X-Kms-Key-Ring,omitempty"`

	// Allows users to set headers on API requests.
	Headers map[string]string
}

// NewSyncAssociatedResourcesOptions : Instantiate SyncAssociatedResourcesOptions
func (*IbmKeyProtectApiV2) NewSyncAssociatedResourcesOptions(id string, bluemixInstance string) *SyncAssociatedResourcesOptions {
	return &SyncAssociatedResourcesOptions{
		ID:              core.StringPtr(id),
		BluemixInstance: core.StringPtr(bluemixInstance),
	}
}

// SetID : Allow user to set ID
func (_options *SyncAssociatedResourcesOptions) SetID(id string) *SyncAssociatedResourcesOptions {
	_options.ID = core.StringPtr(id)
	return _options
}

// SetBluemixInstance : Allow user to set BluemixInstance
func (_options *SyncAssociatedResourcesOptions) SetBluemixInstance(bluemixInstance string) *SyncAssociatedResourcesOptions {
	_options.BluemixInstance = core.StringPtr(bluemixInstance)
	return _options
}

// SetCorrelationID : Allow user to set CorrelationID
func (_options *SyncAssociatedResourcesOptions) SetCorrelationID(correlationID string) *SyncAssociatedResourcesOptions {
	_options.CorrelationID = core.StringPtr(correlationID)
	return _options
}

// SetXKmsKeyRing : Allow user to set XKmsKeyRing
func (_options *SyncAssociatedResourcesOptions) SetXKmsKeyRing(xKmsKeyRing string) *SyncAssociatedResourcesOptions {
	_options.XKmsKeyRing = core.StringPtr(xKmsKeyRing)
	return _options
}

// SetHeaders : Allow user to set Headers
func (options *SyncAssociatedResourcesOptions) SetHeaders(param map[string]string) *SyncAssociatedResourcesOptions {
	options.Headers = param
	return options
}

// UnsetKeyForDeletionOptions : The UnsetKeyForDeletion options.
type UnsetKeyForDeletionOptions struct {
	// The v4 UUID or alias that uniquely identifies the key.
	ID *string `json:"id" validate:"required,ne="`

	// The IBM Cloud instance ID that identifies your Key Protect service instance.
	BluemixInstance *string `json:"Bluemix-Instance" validate:"required"`

	// The v4 UUID used to correlate and track transactions.
	CorrelationID *string `json:"Correlation-Id,omitempty"`

	// The ID of the key ring that the specified key is a part of. When the header is not specified, Key Protect will
	// perform a key ring lookup. For a more optimized request, specify the key ring on every call. The key ring ID of keys
	// that are created without an `X-Kms-Key-Ring` header is: `default`.
	XKmsKeyRing *string `json:"X-Kms-Key-Ring,omitempty"`

	// Allows users to set headers on API requests.
	Headers map[string]string
}

// NewUnsetKeyForDeletionOptions : Instantiate UnsetKeyForDeletionOptions
func (*IbmKeyProtectApiV2) NewUnsetKeyForDeletionOptions(id string, bluemixInstance string) *UnsetKeyForDeletionOptions {
	return &UnsetKeyForDeletionOptions{
		ID:              core.StringPtr(id),
		BluemixInstance: core.StringPtr(bluemixInstance),
	}
}

// SetID : Allow user to set ID
func (_options *UnsetKeyForDeletionOptions) SetID(id string) *UnsetKeyForDeletionOptions {
	_options.ID = core.StringPtr(id)
	return _options
}

// SetBluemixInstance : Allow user to set BluemixInstance
func (_options *UnsetKeyForDeletionOptions) SetBluemixInstance(bluemixInstance string) *UnsetKeyForDeletionOptions {
	_options.BluemixInstance = core.StringPtr(bluemixInstance)
	return _options
}

// SetCorrelationID : Allow user to set CorrelationID
func (_options *UnsetKeyForDeletionOptions) SetCorrelationID(correlationID string) *UnsetKeyForDeletionOptions {
	_options.CorrelationID = core.StringPtr(correlationID)
	return _options
}

// SetXKmsKeyRing : Allow user to set XKmsKeyRing
func (_options *UnsetKeyForDeletionOptions) SetXKmsKeyRing(xKmsKeyRing string) *UnsetKeyForDeletionOptions {
	_options.XKmsKeyRing = core.StringPtr(xKmsKeyRing)
	return _options
}

// SetHeaders : Allow user to set Headers
func (options *UnsetKeyForDeletionOptions) SetHeaders(param map[string]string) *UnsetKeyForDeletionOptions {
	options.Headers = param
	return options
}

// UnwrapKeyOptions : The UnwrapKey options.
type UnwrapKeyOptions struct {
	// The v4 UUID or alias that uniquely identifies the key.
	ID *string `json:"id" validate:"required,ne="`

	// The IBM Cloud instance ID that identifies your Key Protect service instance.
	BluemixInstance *string `json:"Bluemix-Instance" validate:"required"`

	// The base request for unwrap key action.
	KeyActionUnwrapBody io.ReadCloser `json:"KeyActionUnwrapBody" validate:"required"`

	// The v4 UUID used to correlate and track transactions.
	CorrelationID *string `json:"Correlation-Id,omitempty"`

	// The ID of the key ring that the specified key is a part of. When the header is not specified, Key Protect will
	// perform a key ring lookup. For a more optimized request, specify the key ring on every call. The key ring ID of keys
	// that are created without an `X-Kms-Key-Ring` header is: `default`.
	XKmsKeyRing *string `json:"X-Kms-Key-Ring,omitempty"`

	// Allows users to set headers on API requests.
	Headers map[string]string
}

// NewUnwrapKeyOptions : Instantiate UnwrapKeyOptions
func (*IbmKeyProtectApiV2) NewUnwrapKeyOptions(id string, bluemixInstance string, keyActionUnwrapBody io.ReadCloser) *UnwrapKeyOptions {
	return &UnwrapKeyOptions{
		ID:                  core.StringPtr(id),
		BluemixInstance:     core.StringPtr(bluemixInstance),
		KeyActionUnwrapBody: keyActionUnwrapBody,
	}
}

// SetID : Allow user to set ID
func (_options *UnwrapKeyOptions) SetID(id string) *UnwrapKeyOptions {
	_options.ID = core.StringPtr(id)
	return _options
}

// SetBluemixInstance : Allow user to set BluemixInstance
func (_options *UnwrapKeyOptions) SetBluemixInstance(bluemixInstance string) *UnwrapKeyOptions {
	_options.BluemixInstance = core.StringPtr(bluemixInstance)
	return _options
}

// SetKeyActionUnwrapBody : Allow user to set KeyActionUnwrapBody
func (_options *UnwrapKeyOptions) SetKeyActionUnwrapBody(keyActionUnwrapBody io.ReadCloser) *UnwrapKeyOptions {
	_options.KeyActionUnwrapBody = keyActionUnwrapBody
	return _options
}

// SetCorrelationID : Allow user to set CorrelationID
func (_options *UnwrapKeyOptions) SetCorrelationID(correlationID string) *UnwrapKeyOptions {
	_options.CorrelationID = core.StringPtr(correlationID)
	return _options
}

// SetXKmsKeyRing : Allow user to set XKmsKeyRing
func (_options *UnwrapKeyOptions) SetXKmsKeyRing(xKmsKeyRing string) *UnwrapKeyOptions {
	_options.XKmsKeyRing = core.StringPtr(xKmsKeyRing)
	return _options
}

// SetHeaders : Allow user to set Headers
func (options *UnwrapKeyOptions) SetHeaders(param map[string]string) *UnwrapKeyOptions {
	options.Headers = param
	return options
}

// UnwrapKeyResponseBody : Properties that are associated with the response body of an unwrap action.
type UnwrapKeyResponseBody struct {
	// The data encryption key (DEK) used in wrap actions when the query parameter is set to `wrap`. The system returns a
	// base64 encoded plaintext in the response entity-body when you perform an `unwrap` action on a key. To wrap an
	// existing DEK, provide a base64 encoded plaintext during a `wrap` action. To generate a new DEK, omit the `plaintext`
	// property. Key Protect generates a random plaintext (32 bytes) that is rooted in an HSM and then wraps that value.
	// **Note:** When you unwrap a wrapped data encryption key (WDEK) by using a rotated root key, the service returns a
	// new ciphertext in the response entity-body. Each ciphertext remains available for `unwrap` actions. If you unwrap a
	// DEK with a previous ciphertext, the service also returns the latest ciphertext in the response. Use the latest
	// ciphertext for future unwrap operations.
	Plaintext *string `json:"plaintext,omitempty"`

	// The wrapped data encryption key (WDEK) that you can export to your app or service. The ciphertext contains the DEK
	// wrapped by the latest version of the key (WDEK). It is recommended to store and use this WDEK in future calls to Key
	// Protect. The value is base64 encoded.
	Ciphertext *string `json:"ciphertext,omitempty"`

	// The key version that was used to wrap the DEK. This key version is associated with the `ciphertext` value that was
	// used in the request.
	KeyVersion *WrappedKeyVersionKeyVersion `json:"keyVersion,omitempty"`

	// The latest key version that was used to rewrap the DEK. This key version is associated with the `ciphertext` value
	// that's returned in the response.
	RewrappedKeyVersion *RewrappedKeyVersionRewrappedKeyVersion `json:"rewrappedKeyVersion,omitempty"`
}

// UnmarshalUnwrapKeyResponseBody unmarshals an instance of UnwrapKeyResponseBody from the specified map of raw messages.
func UnmarshalUnwrapKeyResponseBody(m map[string]json.RawMessage, result interface{}) (err error) {
	obj := new(UnwrapKeyResponseBody)
	err = core.UnmarshalPrimitive(m, "plaintext", &obj.Plaintext)
	if err != nil {
		err = core.SDKErrorf(err, "", "plaintext-error", common.GetComponentInfo())
		return
	}
	err = core.UnmarshalPrimitive(m, "ciphertext", &obj.Ciphertext)
	if err != nil {
		err = core.SDKErrorf(err, "", "ciphertext-error", common.GetComponentInfo())
		return
	}
	err = core.UnmarshalModel(m, "keyVersion", &obj.KeyVersion, UnmarshalWrappedKeyVersionKeyVersion)
	if err != nil {
		err = core.SDKErrorf(err, "", "keyVersion-error", common.GetComponentInfo())
		return
	}
	err = core.UnmarshalModel(m, "rewrappedKeyVersion", &obj.RewrappedKeyVersion, UnmarshalRewrappedKeyVersionRewrappedKeyVersion)
	if err != nil {
		err = core.SDKErrorf(err, "", "rewrappedKeyVersion-error", common.GetComponentInfo())
		return
	}
	reflect.ValueOf(result).Elem().Set(reflect.ValueOf(obj))
	return
}

// WrapKeyOptions : The WrapKey options.
type WrapKeyOptions struct {
	// The v4 UUID or alias that uniquely identifies the key.
	ID *string `json:"id" validate:"required,ne="`

	// The IBM Cloud instance ID that identifies your Key Protect service instance.
	BluemixInstance *string `json:"Bluemix-Instance" validate:"required"`

	// The base request for wrap key action.
	KeyActionWrapBody io.ReadCloser `json:"KeyActionWrapBody,omitempty"`

	// The v4 UUID used to correlate and track transactions.
	CorrelationID *string `json:"Correlation-Id,omitempty"`

	// The ID of the key ring that the specified key is a part of. When the header is not specified, Key Protect will
	// perform a key ring lookup. For a more optimized request, specify the key ring on every call. The key ring ID of keys
	// that are created without an `X-Kms-Key-Ring` header is: `default`.
	XKmsKeyRing *string `json:"X-Kms-Key-Ring,omitempty"`

	// Allows users to set headers on API requests.
	Headers map[string]string
}

// NewWrapKeyOptions : Instantiate WrapKeyOptions
func (*IbmKeyProtectApiV2) NewWrapKeyOptions(id string, bluemixInstance string) *WrapKeyOptions {
	return &WrapKeyOptions{
		ID:              core.StringPtr(id),
		BluemixInstance: core.StringPtr(bluemixInstance),
	}
}

// SetID : Allow user to set ID
func (_options *WrapKeyOptions) SetID(id string) *WrapKeyOptions {
	_options.ID = core.StringPtr(id)
	return _options
}

// SetBluemixInstance : Allow user to set BluemixInstance
func (_options *WrapKeyOptions) SetBluemixInstance(bluemixInstance string) *WrapKeyOptions {
	_options.BluemixInstance = core.StringPtr(bluemixInstance)
	return _options
}

// SetKeyActionWrapBody : Allow user to set KeyActionWrapBody
func (_options *WrapKeyOptions) SetKeyActionWrapBody(keyActionWrapBody io.ReadCloser) *WrapKeyOptions {
	_options.KeyActionWrapBody = keyActionWrapBody
	return _options
}

// SetCorrelationID : Allow user to set CorrelationID
func (_options *WrapKeyOptions) SetCorrelationID(correlationID string) *WrapKeyOptions {
	_options.CorrelationID = core.StringPtr(correlationID)
	return _options
}

// SetXKmsKeyRing : Allow user to set XKmsKeyRing
func (_options *WrapKeyOptions) SetXKmsKeyRing(xKmsKeyRing string) *WrapKeyOptions {
	_options.XKmsKeyRing = core.StringPtr(xKmsKeyRing)
	return _options
}

// SetHeaders : Allow user to set Headers
func (options *WrapKeyOptions) SetHeaders(param map[string]string) *WrapKeyOptions {
	options.Headers = param
	return options
}

// WrapKeyResponseBody : Properties that are associated with the response body of a wrap action.
type WrapKeyResponseBody struct {
	// The data encryption key (DEK) used in wrap actions when the query parameter is set to `wrap`. The system returns a
	// base64 encoded plaintext in the response entity-body when you perform an `unwrap` action on a key. To wrap an
	// existing DEK, provide a base64 encoded plaintext during a `wrap` action. To generate a new DEK, omit the `plaintext`
	// property. Key Protect generates a random plaintext (32 bytes) that is rooted in an HSM and then wraps that value.
	// **Note:** When you unwrap a wrapped data encryption key (WDEK) by using a rotated root key, the service returns a
	// new ciphertext in the response entity-body. Each ciphertext remains available for `unwrap` actions. If you unwrap a
	// DEK with a previous ciphertext, the service also returns the latest ciphertext in the response. Use the latest
	// ciphertext for future unwrap operations.
	Plaintext *string `json:"plaintext,omitempty"`

	// The wrapped data encryption key (WDEK) that you can export to your app or service. The ciphertext contains the DEK
	// wrapped by the latest version of the key (WDEK). It is recommended to store and use this WDEK in future calls to Key
	// Protect. The value is base64 encoded.
	Ciphertext *string `json:"ciphertext,omitempty"`

	// The key version that was used to wrap the DEK. This key version is associated with the `ciphertext` value that was
	// used in the request.
	KeyVersion *WrappedKeyVersionKeyVersion `json:"keyVersion,omitempty"`
}

// UnmarshalWrapKeyResponseBody unmarshals an instance of WrapKeyResponseBody from the specified map of raw messages.
func UnmarshalWrapKeyResponseBody(m map[string]json.RawMessage, result interface{}) (err error) {
	obj := new(WrapKeyResponseBody)
	err = core.UnmarshalPrimitive(m, "plaintext", &obj.Plaintext)
	if err != nil {
		err = core.SDKErrorf(err, "", "plaintext-error", common.GetComponentInfo())
		return
	}
	err = core.UnmarshalPrimitive(m, "ciphertext", &obj.Ciphertext)
	if err != nil {
		err = core.SDKErrorf(err, "", "ciphertext-error", common.GetComponentInfo())
		return
	}
	err = core.UnmarshalModel(m, "keyVersion", &obj.KeyVersion, UnmarshalWrappedKeyVersionKeyVersion)
	if err != nil {
		err = core.SDKErrorf(err, "", "keyVersion-error", common.GetComponentInfo())
		return
	}
	reflect.ValueOf(result).Elem().Set(reflect.ValueOf(obj))
	return
}

// WrappedKeyVersionKeyVersion : The key version that was used to wrap the DEK. This key version is associated with the `ciphertext` value that was
// used in the request.
type WrappedKeyVersionKeyVersion struct {
	// The ID of the key version.
	ID *string `json:"id,omitempty"`
}

// UnmarshalWrappedKeyVersionKeyVersion unmarshals an instance of WrappedKeyVersionKeyVersion from the specified map of raw messages.
func UnmarshalWrappedKeyVersionKeyVersion(m map[string]json.RawMessage, result interface{}) (err error) {
	obj := new(WrappedKeyVersionKeyVersion)
	err = core.UnmarshalPrimitive(m, "id", &obj.ID)
	if err != nil {
		err = core.SDKErrorf(err, "", "id-error", common.GetComponentInfo())
		return
	}
	reflect.ValueOf(result).Elem().Set(reflect.ValueOf(obj))
	return
}

// CollectionMetadataOneOfCollectionMetadata : The metadata that describes the resource array.
// This model "extends" CollectionMetadataOneOf
type CollectionMetadataOneOfCollectionMetadata struct {
	// The type of resources in the resource array.
	CollectionType *string `json:"collectionType" validate:"required"`

	// The number of elements in the resource array.
	CollectionTotal *int64 `json:"collectionTotal" validate:"required"`
}

// Constants associated with the CollectionMetadataOneOfCollectionMetadata.CollectionType property.
// The type of resources in the resource array.
const (
	CollectionMetadataOneOfCollectionMetadata_CollectionType_ApplicationVndIbmKmsAliasJSON                 = "application/vnd.ibm.kms.alias+json"
	CollectionMetadataOneOfCollectionMetadata_CollectionType_ApplicationVndIbmKmsAllowedIpMetadataJSON     = "application/vnd.ibm.kms.allowed_ip_metadata+json"
	CollectionMetadataOneOfCollectionMetadata_CollectionType_ApplicationVndIbmKmsCrnJSON                   = "application/vnd.ibm.kms.crn+json"
	CollectionMetadataOneOfCollectionMetadata_CollectionType_ApplicationVndIbmKmsErrorJSON                 = "application/vnd.ibm.kms.error+json"
	CollectionMetadataOneOfCollectionMetadata_CollectionType_ApplicationVndIbmKmsEventAcknowledgeJSON      = "application/vnd.ibm.kms.event_acknowledge+json"
	CollectionMetadataOneOfCollectionMetadata_CollectionType_ApplicationVndIbmKmsImportTokenJSON           = "application/vnd.ibm.kms.import_token+json"
	CollectionMetadataOneOfCollectionMetadata_CollectionType_ApplicationVndIbmKmsKeyActionJSON             = "application/vnd.ibm.kms.key_action+json"
	CollectionMetadataOneOfCollectionMetadata_CollectionType_ApplicationVndIbmKmsKeyJSON                   = "application/vnd.ibm.kms.key+json"
	CollectionMetadataOneOfCollectionMetadata_CollectionType_ApplicationVndIbmKmsKeyRingJSON               = "application/vnd.ibm.kms.key_ring+json"
	CollectionMetadataOneOfCollectionMetadata_CollectionType_ApplicationVndIbmKmsKmipAdapterJSON           = "application/vnd.ibm.kms.kmip_adapter+json"
	CollectionMetadataOneOfCollectionMetadata_CollectionType_ApplicationVndIbmKmsKmipClientCertificateJSON = "application/vnd.ibm.kms.kmip_client_certificate+json"
	CollectionMetadataOneOfCollectionMetadata_CollectionType_ApplicationVndIbmKmsKmipObjectJSON            = "application/vnd.ibm.kms.kmip_object+json"
	CollectionMetadataOneOfCollectionMetadata_CollectionType_ApplicationVndIbmKmsPolicyJSON                = "application/vnd.ibm.kms.policy+json"
	CollectionMetadataOneOfCollectionMetadata_CollectionType_ApplicationVndIbmKmsRegistrationInputJSON     = "application/vnd.ibm.kms.registration_input+json"
	CollectionMetadataOneOfCollectionMetadata_CollectionType_ApplicationVndIbmKmsRegistrationJSON          = "application/vnd.ibm.kms.registration+json"
	CollectionMetadataOneOfCollectionMetadata_CollectionType_ApplicationVndIbmKmsResourceCrnJSON           = "application/vnd.ibm.kms.resource_crn+json"
)

func (*CollectionMetadataOneOfCollectionMetadata) isaCollectionMetadataOneOf() bool {
	return true
}

// UnmarshalCollectionMetadataOneOfCollectionMetadata unmarshals an instance of CollectionMetadataOneOfCollectionMetadata from the specified map of raw messages.
func UnmarshalCollectionMetadataOneOfCollectionMetadata(m map[string]json.RawMessage, result interface{}) (err error) {
	obj := new(CollectionMetadataOneOfCollectionMetadata)
	err = core.UnmarshalPrimitive(m, "collectionType", &obj.CollectionType)
	if err != nil {
		err = core.SDKErrorf(err, "", "collectionType-error", common.GetComponentInfo())
		return
	}
	err = core.UnmarshalPrimitive(m, "collectionTotal", &obj.CollectionTotal)
	if err != nil {
		err = core.SDKErrorf(err, "", "collectionTotal-error", common.GetComponentInfo())
		return
	}
	reflect.ValueOf(result).Elem().Set(reflect.ValueOf(obj))
	return
}

// GetInstancePoliciesOneOfGetInstancePolicyAllowedIP : Properties that are associated with retrieving an instance level allowed IP policy.
// This model "extends" GetInstancePoliciesOneOf
type GetInstancePoliciesOneOfGetInstancePolicyAllowedIP struct {
	Metadata CollectionMetadataOneOfIntf `json:"metadata" validate:"required"`

	// A collection of resources.
	Resources []GetInstancePolicyAllowedIPResourcesItem `json:"resources" validate:"required"`
}

func (*GetInstancePoliciesOneOfGetInstancePolicyAllowedIP) isaGetInstancePoliciesOneOf() bool {
	return true
}

// UnmarshalGetInstancePoliciesOneOfGetInstancePolicyAllowedIP unmarshals an instance of GetInstancePoliciesOneOfGetInstancePolicyAllowedIP from the specified map of raw messages.
func UnmarshalGetInstancePoliciesOneOfGetInstancePolicyAllowedIP(m map[string]json.RawMessage, result interface{}) (err error) {
	obj := new(GetInstancePoliciesOneOfGetInstancePolicyAllowedIP)
	err = core.UnmarshalModel(m, "metadata", &obj.Metadata, UnmarshalCollectionMetadataOneOf)
	if err != nil {
		err = core.SDKErrorf(err, "", "metadata-error", common.GetComponentInfo())
		return
	}
	err = core.UnmarshalModel(m, "resources", &obj.Resources, UnmarshalGetInstancePolicyAllowedIPResourcesItem)
	if err != nil {
		err = core.SDKErrorf(err, "", "resources-error", common.GetComponentInfo())
		return
	}
	reflect.ValueOf(result).Elem().Set(reflect.ValueOf(obj))
	return
}

// GetInstancePoliciesOneOfGetInstancePolicyAllowedNetwork : Properties that are associated with retrieving an instance level allowed network policy.
// This model "extends" GetInstancePoliciesOneOf
type GetInstancePoliciesOneOfGetInstancePolicyAllowedNetwork struct {
	Metadata CollectionMetadataOneOfIntf `json:"metadata" validate:"required"`

	// A collection of resources.
	Resources []GetInstancePoliciesOneOfGetInstancePolicyAllowedNetworkResourcesItem `json:"resources" validate:"required"`
}

func (*GetInstancePoliciesOneOfGetInstancePolicyAllowedNetwork) isaGetInstancePoliciesOneOf() bool {
	return true
}

// UnmarshalGetInstancePoliciesOneOfGetInstancePolicyAllowedNetwork unmarshals an instance of GetInstancePoliciesOneOfGetInstancePolicyAllowedNetwork from the specified map of raw messages.
func UnmarshalGetInstancePoliciesOneOfGetInstancePolicyAllowedNetwork(m map[string]json.RawMessage, result interface{}) (err error) {
	obj := new(GetInstancePoliciesOneOfGetInstancePolicyAllowedNetwork)
	err = core.UnmarshalModel(m, "metadata", &obj.Metadata, UnmarshalCollectionMetadataOneOf)
	if err != nil {
		err = core.SDKErrorf(err, "", "metadata-error", common.GetComponentInfo())
		return
	}
	err = core.UnmarshalModel(m, "resources", &obj.Resources, UnmarshalGetInstancePoliciesOneOfGetInstancePolicyAllowedNetworkResourcesItem)
	if err != nil {
		err = core.SDKErrorf(err, "", "resources-error", common.GetComponentInfo())
		return
	}
	reflect.ValueOf(result).Elem().Set(reflect.ValueOf(obj))
	return
}

// GetInstancePoliciesOneOfGetInstancePolicyDualAuthDelete : Properties that are associated with retrieving an instance level dual authorization delete policy.
// This model "extends" GetInstancePoliciesOneOf
type GetInstancePoliciesOneOfGetInstancePolicyDualAuthDelete struct {
	Metadata CollectionMetadataOneOfIntf `json:"metadata" validate:"required"`

	// A collection of resources.
	Resources []GetInstancePolicyDualAuthDeleteResourcesItem `json:"resources" validate:"required"`
}

func (*GetInstancePoliciesOneOfGetInstancePolicyDualAuthDelete) isaGetInstancePoliciesOneOf() bool {
	return true
}

// UnmarshalGetInstancePoliciesOneOfGetInstancePolicyDualAuthDelete unmarshals an instance of GetInstancePoliciesOneOfGetInstancePolicyDualAuthDelete from the specified map of raw messages.
func UnmarshalGetInstancePoliciesOneOfGetInstancePolicyDualAuthDelete(m map[string]json.RawMessage, result interface{}) (err error) {
	obj := new(GetInstancePoliciesOneOfGetInstancePolicyDualAuthDelete)
	err = core.UnmarshalModel(m, "metadata", &obj.Metadata, UnmarshalCollectionMetadataOneOf)
	if err != nil {
		err = core.SDKErrorf(err, "", "metadata-error", common.GetComponentInfo())
		return
	}
	err = core.UnmarshalModel(m, "resources", &obj.Resources, UnmarshalGetInstancePolicyDualAuthDeleteResourcesItem)
	if err != nil {
		err = core.SDKErrorf(err, "", "resources-error", common.GetComponentInfo())
		return
	}
	reflect.ValueOf(result).Elem().Set(reflect.ValueOf(obj))
	return
}

// GetInstancePoliciesOneOfGetInstancePolicyKeyCreateImportAccess : Properties that are associated with retrieving an instance level key create and import access policy.
// This model "extends" GetInstancePoliciesOneOf
type GetInstancePoliciesOneOfGetInstancePolicyKeyCreateImportAccess struct {
	Metadata CollectionMetadataOneOfIntf `json:"metadata" validate:"required"`

	// A collection of resources.
	Resources []GetInstancePoliciesOneOfGetInstancePolicyKeyCreateImportAccessResourcesItem `json:"resources" validate:"required"`
}

func (*GetInstancePoliciesOneOfGetInstancePolicyKeyCreateImportAccess) isaGetInstancePoliciesOneOf() bool {
	return true
}

// UnmarshalGetInstancePoliciesOneOfGetInstancePolicyKeyCreateImportAccess unmarshals an instance of GetInstancePoliciesOneOfGetInstancePolicyKeyCreateImportAccess from the specified map of raw messages.
func UnmarshalGetInstancePoliciesOneOfGetInstancePolicyKeyCreateImportAccess(m map[string]json.RawMessage, result interface{}) (err error) {
	obj := new(GetInstancePoliciesOneOfGetInstancePolicyKeyCreateImportAccess)
	err = core.UnmarshalModel(m, "metadata", &obj.Metadata, UnmarshalCollectionMetadataOneOf)
	if err != nil {
		err = core.SDKErrorf(err, "", "metadata-error", common.GetComponentInfo())
		return
	}
	err = core.UnmarshalModel(m, "resources", &obj.Resources, UnmarshalGetInstancePoliciesOneOfGetInstancePolicyKeyCreateImportAccessResourcesItem)
	if err != nil {
		err = core.SDKErrorf(err, "", "resources-error", common.GetComponentInfo())
		return
	}
	reflect.ValueOf(result).Elem().Set(reflect.ValueOf(obj))
	return
}

// GetInstancePoliciesOneOfGetInstancePolicyMetrics : Properties that are associated with retrieving an instance level metrics policy.
// This model "extends" GetInstancePoliciesOneOf
type GetInstancePoliciesOneOfGetInstancePolicyMetrics struct {
	Metadata CollectionMetadataOneOfIntf `json:"metadata" validate:"required"`

	// A collection of resources.
	Resources []GetInstancePolicyMetricsResourcesItem `json:"resources" validate:"required"`
}

func (*GetInstancePoliciesOneOfGetInstancePolicyMetrics) isaGetInstancePoliciesOneOf() bool {
	return true
}

// UnmarshalGetInstancePoliciesOneOfGetInstancePolicyMetrics unmarshals an instance of GetInstancePoliciesOneOfGetInstancePolicyMetrics from the specified map of raw messages.
func UnmarshalGetInstancePoliciesOneOfGetInstancePolicyMetrics(m map[string]json.RawMessage, result interface{}) (err error) {
	obj := new(GetInstancePoliciesOneOfGetInstancePolicyMetrics)
	err = core.UnmarshalModel(m, "metadata", &obj.Metadata, UnmarshalCollectionMetadataOneOf)
	if err != nil {
		err = core.SDKErrorf(err, "", "metadata-error", common.GetComponentInfo())
		return
	}
	err = core.UnmarshalModel(m, "resources", &obj.Resources, UnmarshalGetInstancePolicyMetricsResourcesItem)
	if err != nil {
		err = core.SDKErrorf(err, "", "resources-error", common.GetComponentInfo())
		return
	}
	reflect.ValueOf(result).Elem().Set(reflect.ValueOf(obj))
	return
}

// GetInstancePoliciesOneOfGetInstancePolicyRotation : Properties that are associated with retrieving an instance level rotation policy.
// This model "extends" GetInstancePoliciesOneOf
type GetInstancePoliciesOneOfGetInstancePolicyRotation struct {
	Metadata CollectionMetadataOneOfIntf `json:"metadata" validate:"required"`

	// A collection of resources.
	Resources []GetInstancePolicyRotationResourcesItem `json:"resources" validate:"required"`
}

func (*GetInstancePoliciesOneOfGetInstancePolicyRotation) isaGetInstancePoliciesOneOf() bool {
	return true
}

// UnmarshalGetInstancePoliciesOneOfGetInstancePolicyRotation unmarshals an instance of GetInstancePoliciesOneOfGetInstancePolicyRotation from the specified map of raw messages.
func UnmarshalGetInstancePoliciesOneOfGetInstancePolicyRotation(m map[string]json.RawMessage, result interface{}) (err error) {
	obj := new(GetInstancePoliciesOneOfGetInstancePolicyRotation)
	err = core.UnmarshalModel(m, "metadata", &obj.Metadata, UnmarshalCollectionMetadataOneOf)
	if err != nil {
		err = core.SDKErrorf(err, "", "metadata-error", common.GetComponentInfo())
		return
	}
	err = core.UnmarshalModel(m, "resources", &obj.Resources, UnmarshalGetInstancePolicyRotationResourcesItem)
	if err != nil {
		err = core.SDKErrorf(err, "", "resources-error", common.GetComponentInfo())
		return
	}
	reflect.ValueOf(result).Elem().Set(reflect.ValueOf(obj))
	return
}

// GetInstancePoliciesOneOfGetMultipleInstancePolicies : Properties that are associated with the instance level policies.
// This model "extends" GetInstancePoliciesOneOf
type GetInstancePoliciesOneOfGetMultipleInstancePolicies struct {
	Metadata CollectionMetadataOneOfIntf `json:"metadata" validate:"required"`

	// A collection of resources.
	Resources []InstancePolicyResource `json:"resources" validate:"required"`
}

func (*GetInstancePoliciesOneOfGetMultipleInstancePolicies) isaGetInstancePoliciesOneOf() bool {
	return true
}

// UnmarshalGetInstancePoliciesOneOfGetMultipleInstancePolicies unmarshals an instance of GetInstancePoliciesOneOfGetMultipleInstancePolicies from the specified map of raw messages.
func UnmarshalGetInstancePoliciesOneOfGetMultipleInstancePolicies(m map[string]json.RawMessage, result interface{}) (err error) {
	obj := new(GetInstancePoliciesOneOfGetMultipleInstancePolicies)
	err = core.UnmarshalModel(m, "metadata", &obj.Metadata, UnmarshalCollectionMetadataOneOf)
	if err != nil {
		err = core.SDKErrorf(err, "", "metadata-error", common.GetComponentInfo())
		return
	}
	err = core.UnmarshalModel(m, "resources", &obj.Resources, UnmarshalInstancePolicyResource)
	if err != nil {
		err = core.SDKErrorf(err, "", "resources-error", common.GetComponentInfo())
		return
	}
	reflect.ValueOf(result).Elem().Set(reflect.ValueOf(obj))
	return
}

// GetKeyPoliciesOneOfGetKeyPolicyDualAuthDelete : The base schema for retrieving a dual authorization key policy.
// This model "extends" GetKeyPoliciesOneOf
type GetKeyPoliciesOneOfGetKeyPolicyDualAuthDelete struct {
	// The metadata that describes the resource array.
	Metadata *CollectionMetadata `json:"metadata" validate:"required"`

	// A collection of resources.
	Resources []GetKeyPoliciesOneOfGetKeyPolicyDualAuthDeleteResourcesItem `json:"resources" validate:"required"`
}

func (*GetKeyPoliciesOneOfGetKeyPolicyDualAuthDelete) isaGetKeyPoliciesOneOf() bool {
	return true
}

// UnmarshalGetKeyPoliciesOneOfGetKeyPolicyDualAuthDelete unmarshals an instance of GetKeyPoliciesOneOfGetKeyPolicyDualAuthDelete from the specified map of raw messages.
func UnmarshalGetKeyPoliciesOneOfGetKeyPolicyDualAuthDelete(m map[string]json.RawMessage, result interface{}) (err error) {
	obj := new(GetKeyPoliciesOneOfGetKeyPolicyDualAuthDelete)
	err = core.UnmarshalModel(m, "metadata", &obj.Metadata, UnmarshalCollectionMetadata)
	if err != nil {
		err = core.SDKErrorf(err, "", "metadata-error", common.GetComponentInfo())
		return
	}
	err = core.UnmarshalModel(m, "resources", &obj.Resources, UnmarshalGetKeyPoliciesOneOfGetKeyPolicyDualAuthDeleteResourcesItem)
	if err != nil {
		err = core.SDKErrorf(err, "", "resources-error", common.GetComponentInfo())
		return
	}
	reflect.ValueOf(result).Elem().Set(reflect.ValueOf(obj))
	return
}

// GetKeyPoliciesOneOfGetKeyPolicyRotation : The base schema for retrieving a dual authorization key policy.
// This model "extends" GetKeyPoliciesOneOf
type GetKeyPoliciesOneOfGetKeyPolicyRotation struct {
	// The metadata that describes the resource array.
	Metadata *CollectionMetadata `json:"metadata" validate:"required"`

	// A collection of resources.
	Resources []GetKeyPolicyRotationResourcesItem `json:"resources" validate:"required"`
}

func (*GetKeyPoliciesOneOfGetKeyPolicyRotation) isaGetKeyPoliciesOneOf() bool {
	return true
}

// UnmarshalGetKeyPoliciesOneOfGetKeyPolicyRotation unmarshals an instance of GetKeyPoliciesOneOfGetKeyPolicyRotation from the specified map of raw messages.
func UnmarshalGetKeyPoliciesOneOfGetKeyPolicyRotation(m map[string]json.RawMessage, result interface{}) (err error) {
	obj := new(GetKeyPoliciesOneOfGetKeyPolicyRotation)
	err = core.UnmarshalModel(m, "metadata", &obj.Metadata, UnmarshalCollectionMetadata)
	if err != nil {
		err = core.SDKErrorf(err, "", "metadata-error", common.GetComponentInfo())
		return
	}
	err = core.UnmarshalModel(m, "resources", &obj.Resources, UnmarshalGetKeyPolicyRotationResourcesItem)
	if err != nil {
		err = core.SDKErrorf(err, "", "resources-error", common.GetComponentInfo())
		return
	}
	reflect.ValueOf(result).Elem().Set(reflect.ValueOf(obj))
	return
}

// GetKeyPoliciesOneOfGetMultipleKeyPolicies : The base schema for retrieving all key policies.
// This model "extends" GetKeyPoliciesOneOf
type GetKeyPoliciesOneOfGetMultipleKeyPolicies struct {
	// The metadata that describes the resource array.
	Metadata *CollectionMetadata `json:"metadata" validate:"required"`

	// A collection of resources.
	Resources []GetMultipleKeyPoliciesResource `json:"resources" validate:"required"`
}

func (*GetKeyPoliciesOneOfGetMultipleKeyPolicies) isaGetKeyPoliciesOneOf() bool {
	return true
}

// UnmarshalGetKeyPoliciesOneOfGetMultipleKeyPolicies unmarshals an instance of GetKeyPoliciesOneOfGetMultipleKeyPolicies from the specified map of raw messages.
func UnmarshalGetKeyPoliciesOneOfGetMultipleKeyPolicies(m map[string]json.RawMessage, result interface{}) (err error) {
	obj := new(GetKeyPoliciesOneOfGetMultipleKeyPolicies)
	err = core.UnmarshalModel(m, "metadata", &obj.Metadata, UnmarshalCollectionMetadata)
	if err != nil {
		err = core.SDKErrorf(err, "", "metadata-error", common.GetComponentInfo())
		return
	}
	err = core.UnmarshalModel(m, "resources", &obj.Resources, UnmarshalGetMultipleKeyPoliciesResource)
	if err != nil {
		err = core.SDKErrorf(err, "", "resources-error", common.GetComponentInfo())
		return
	}
	reflect.ValueOf(result).Elem().Set(reflect.ValueOf(obj))
	return
}

// KMIPProfileDataBodyKMIPProfileDataNative : Properties that must be specified to profile_data when it is of native_1.0 KMIP adapter resource.
// This model "extends" KMIPProfileDataBody
type KMIPProfileDataBodyKMIPProfileDataNative struct {
	// An ID that identifies the Customer Root Key(CRK) to be used. This CRK must exist in the same kms instance as the
	// adapter.
	CrkID *string `json:"crk_id" validate:"required"`
}

// NewKMIPProfileDataBodyKMIPProfileDataNative : Instantiate KMIPProfileDataBodyKMIPProfileDataNative (Generic Model Constructor)
func (*IbmKeyProtectApiV2) NewKMIPProfileDataBodyKMIPProfileDataNative(crkID string) (_model *KMIPProfileDataBodyKMIPProfileDataNative, err error) {
	_model = &KMIPProfileDataBodyKMIPProfileDataNative{
		CrkID: core.StringPtr(crkID),
	}
	err = core.ValidateStruct(_model, "required parameters")
	if err != nil {
		err = core.SDKErrorf(err, "", "model-missing-required", common.GetComponentInfo())
	}
	return
}

func (*KMIPProfileDataBodyKMIPProfileDataNative) isaKMIPProfileDataBody() bool {
	return true
}

// UnmarshalKMIPProfileDataBodyKMIPProfileDataNative unmarshals an instance of KMIPProfileDataBodyKMIPProfileDataNative from the specified map of raw messages.
func UnmarshalKMIPProfileDataBodyKMIPProfileDataNative(m map[string]json.RawMessage, result interface{}) (err error) {
	obj := new(KMIPProfileDataBodyKMIPProfileDataNative)
	err = core.UnmarshalPrimitive(m, "crk_id", &obj.CrkID)
	if err != nil {
		err = core.SDKErrorf(err, "", "crk_id-error", common.GetComponentInfo())
		return
	}
	reflect.ValueOf(result).Elem().Set(reflect.ValueOf(obj))
	return
}

// KeyActionOneOfResponseRewrapKeyResponseBody : Properties that are associated with the response body of an rewrap action.
// This model "extends" KeyActionOneOfResponse
type KeyActionOneOfResponseRewrapKeyResponseBody struct {
	// The wrapped data encryption key (WDEK) that you can export to your app or service. The ciphertext contains the DEK
	// wrapped by the latest version of the key (WDEK). It is recommended to store and use this WDEK in future calls to Key
	// Protect. The value is base64 encoded.
	Ciphertext *string `json:"ciphertext,omitempty"`

	// The key version that was used to wrap the DEK. This key version is associated with the `ciphertext` value that was
	// used in the request.
	KeyVersion *WrappedKeyVersionKeyVersion `json:"keyVersion,omitempty"`

	// The latest key version that was used to rewrap the DEK. This key version is associated with the `ciphertext` value
	// that's returned in the response.
	RewrappedKeyVersion *RewrappedKeyVersionRewrappedKeyVersion `json:"rewrappedKeyVersion,omitempty"`
}

func (*KeyActionOneOfResponseRewrapKeyResponseBody) isaKeyActionOneOfResponse() bool {
	return true
}

// UnmarshalKeyActionOneOfResponseRewrapKeyResponseBody unmarshals an instance of KeyActionOneOfResponseRewrapKeyResponseBody from the specified map of raw messages.
func UnmarshalKeyActionOneOfResponseRewrapKeyResponseBody(m map[string]json.RawMessage, result interface{}) (err error) {
	obj := new(KeyActionOneOfResponseRewrapKeyResponseBody)
	err = core.UnmarshalPrimitive(m, "ciphertext", &obj.Ciphertext)
	if err != nil {
		err = core.SDKErrorf(err, "", "ciphertext-error", common.GetComponentInfo())
		return
	}
	err = core.UnmarshalModel(m, "keyVersion", &obj.KeyVersion, UnmarshalWrappedKeyVersionKeyVersion)
	if err != nil {
		err = core.SDKErrorf(err, "", "keyVersion-error", common.GetComponentInfo())
		return
	}
	err = core.UnmarshalModel(m, "rewrappedKeyVersion", &obj.RewrappedKeyVersion, UnmarshalRewrappedKeyVersionRewrappedKeyVersion)
	if err != nil {
		err = core.SDKErrorf(err, "", "rewrappedKeyVersion-error", common.GetComponentInfo())
		return
	}
	reflect.ValueOf(result).Elem().Set(reflect.ValueOf(obj))
	return
}

// KeyActionOneOfResponseUnwrapKeyResponseBody : Properties that are associated with the response body of an unwrap action.
// This model "extends" KeyActionOneOfResponse
type KeyActionOneOfResponseUnwrapKeyResponseBody struct {
	// The data encryption key (DEK) used in wrap actions when the query parameter is set to `wrap`. The system returns a
	// base64 encoded plaintext in the response entity-body when you perform an `unwrap` action on a key. To wrap an
	// existing DEK, provide a base64 encoded plaintext during a `wrap` action. To generate a new DEK, omit the `plaintext`
	// property. Key Protect generates a random plaintext (32 bytes) that is rooted in an HSM and then wraps that value.
	// **Note:** When you unwrap a wrapped data encryption key (WDEK) by using a rotated root key, the service returns a
	// new ciphertext in the response entity-body. Each ciphertext remains available for `unwrap` actions. If you unwrap a
	// DEK with a previous ciphertext, the service also returns the latest ciphertext in the response. Use the latest
	// ciphertext for future unwrap operations.
	Plaintext *string `json:"plaintext,omitempty"`

	// The wrapped data encryption key (WDEK) that you can export to your app or service. The ciphertext contains the DEK
	// wrapped by the latest version of the key (WDEK). It is recommended to store and use this WDEK in future calls to Key
	// Protect. The value is base64 encoded.
	Ciphertext *string `json:"ciphertext,omitempty"`

	// The key version that was used to wrap the DEK. This key version is associated with the `ciphertext` value that was
	// used in the request.
	KeyVersion *WrappedKeyVersionKeyVersion `json:"keyVersion,omitempty"`

	// The latest key version that was used to rewrap the DEK. This key version is associated with the `ciphertext` value
	// that's returned in the response.
	RewrappedKeyVersion *RewrappedKeyVersionRewrappedKeyVersion `json:"rewrappedKeyVersion,omitempty"`
}

func (*KeyActionOneOfResponseUnwrapKeyResponseBody) isaKeyActionOneOfResponse() bool {
	return true
}

// UnmarshalKeyActionOneOfResponseUnwrapKeyResponseBody unmarshals an instance of KeyActionOneOfResponseUnwrapKeyResponseBody from the specified map of raw messages.
func UnmarshalKeyActionOneOfResponseUnwrapKeyResponseBody(m map[string]json.RawMessage, result interface{}) (err error) {
	obj := new(KeyActionOneOfResponseUnwrapKeyResponseBody)
	err = core.UnmarshalPrimitive(m, "plaintext", &obj.Plaintext)
	if err != nil {
		err = core.SDKErrorf(err, "", "plaintext-error", common.GetComponentInfo())
		return
	}
	err = core.UnmarshalPrimitive(m, "ciphertext", &obj.Ciphertext)
	if err != nil {
		err = core.SDKErrorf(err, "", "ciphertext-error", common.GetComponentInfo())
		return
	}
	err = core.UnmarshalModel(m, "keyVersion", &obj.KeyVersion, UnmarshalWrappedKeyVersionKeyVersion)
	if err != nil {
		err = core.SDKErrorf(err, "", "keyVersion-error", common.GetComponentInfo())
		return
	}
	err = core.UnmarshalModel(m, "rewrappedKeyVersion", &obj.RewrappedKeyVersion, UnmarshalRewrappedKeyVersionRewrappedKeyVersion)
	if err != nil {
		err = core.SDKErrorf(err, "", "rewrappedKeyVersion-error", common.GetComponentInfo())
		return
	}
	reflect.ValueOf(result).Elem().Set(reflect.ValueOf(obj))
	return
}

// KeyActionOneOfResponseWrapKeyResponseBody : Properties that are associated with the response body of a wrap action.
// This model "extends" KeyActionOneOfResponse
type KeyActionOneOfResponseWrapKeyResponseBody struct {
	// The data encryption key (DEK) used in wrap actions when the query parameter is set to `wrap`. The system returns a
	// base64 encoded plaintext in the response entity-body when you perform an `unwrap` action on a key. To wrap an
	// existing DEK, provide a base64 encoded plaintext during a `wrap` action. To generate a new DEK, omit the `plaintext`
	// property. Key Protect generates a random plaintext (32 bytes) that is rooted in an HSM and then wraps that value.
	// **Note:** When you unwrap a wrapped data encryption key (WDEK) by using a rotated root key, the service returns a
	// new ciphertext in the response entity-body. Each ciphertext remains available for `unwrap` actions. If you unwrap a
	// DEK with a previous ciphertext, the service also returns the latest ciphertext in the response. Use the latest
	// ciphertext for future unwrap operations.
	Plaintext *string `json:"plaintext,omitempty"`

	// The wrapped data encryption key (WDEK) that you can export to your app or service. The ciphertext contains the DEK
	// wrapped by the latest version of the key (WDEK). It is recommended to store and use this WDEK in future calls to Key
	// Protect. The value is base64 encoded.
	Ciphertext *string `json:"ciphertext,omitempty"`

	// The key version that was used to wrap the DEK. This key version is associated with the `ciphertext` value that was
	// used in the request.
	KeyVersion *WrappedKeyVersionKeyVersion `json:"keyVersion,omitempty"`
}

func (*KeyActionOneOfResponseWrapKeyResponseBody) isaKeyActionOneOfResponse() bool {
	return true
}

// UnmarshalKeyActionOneOfResponseWrapKeyResponseBody unmarshals an instance of KeyActionOneOfResponseWrapKeyResponseBody from the specified map of raw messages.
func UnmarshalKeyActionOneOfResponseWrapKeyResponseBody(m map[string]json.RawMessage, result interface{}) (err error) {
	obj := new(KeyActionOneOfResponseWrapKeyResponseBody)
	err = core.UnmarshalPrimitive(m, "plaintext", &obj.Plaintext)
	if err != nil {
		err = core.SDKErrorf(err, "", "plaintext-error", common.GetComponentInfo())
		return
	}
	err = core.UnmarshalPrimitive(m, "ciphertext", &obj.Ciphertext)
	if err != nil {
		err = core.SDKErrorf(err, "", "ciphertext-error", common.GetComponentInfo())
		return
	}
	err = core.UnmarshalModel(m, "keyVersion", &obj.KeyVersion, UnmarshalWrappedKeyVersionKeyVersion)
	if err != nil {
		err = core.SDKErrorf(err, "", "keyVersion-error", common.GetComponentInfo())
		return
	}
	reflect.ValueOf(result).Elem().Set(reflect.ValueOf(obj))
	return
}

// ListCollectionMetadataCollectionMetadata : The metadata that describes the resource array.
// This model "extends" ListCollectionMetadata
type ListCollectionMetadataCollectionMetadata struct {
	// The type of resources in the resource array.
	CollectionType *string `json:"collectionType" validate:"required"`

	// The number of elements in the resource array.
	CollectionTotal *int64 `json:"collectionTotal" validate:"required"`
}

// Constants associated with the ListCollectionMetadataCollectionMetadata.CollectionType property.
// The type of resources in the resource array.
const (
	ListCollectionMetadataCollectionMetadata_CollectionType_ApplicationVndIbmKmsAliasJSON                 = "application/vnd.ibm.kms.alias+json"
	ListCollectionMetadataCollectionMetadata_CollectionType_ApplicationVndIbmKmsAllowedIpMetadataJSON     = "application/vnd.ibm.kms.allowed_ip_metadata+json"
	ListCollectionMetadataCollectionMetadata_CollectionType_ApplicationVndIbmKmsCrnJSON                   = "application/vnd.ibm.kms.crn+json"
	ListCollectionMetadataCollectionMetadata_CollectionType_ApplicationVndIbmKmsErrorJSON                 = "application/vnd.ibm.kms.error+json"
	ListCollectionMetadataCollectionMetadata_CollectionType_ApplicationVndIbmKmsEventAcknowledgeJSON      = "application/vnd.ibm.kms.event_acknowledge+json"
	ListCollectionMetadataCollectionMetadata_CollectionType_ApplicationVndIbmKmsImportTokenJSON           = "application/vnd.ibm.kms.import_token+json"
	ListCollectionMetadataCollectionMetadata_CollectionType_ApplicationVndIbmKmsKeyActionJSON             = "application/vnd.ibm.kms.key_action+json"
	ListCollectionMetadataCollectionMetadata_CollectionType_ApplicationVndIbmKmsKeyJSON                   = "application/vnd.ibm.kms.key+json"
	ListCollectionMetadataCollectionMetadata_CollectionType_ApplicationVndIbmKmsKeyRingJSON               = "application/vnd.ibm.kms.key_ring+json"
	ListCollectionMetadataCollectionMetadata_CollectionType_ApplicationVndIbmKmsKmipAdapterJSON           = "application/vnd.ibm.kms.kmip_adapter+json"
	ListCollectionMetadataCollectionMetadata_CollectionType_ApplicationVndIbmKmsKmipClientCertificateJSON = "application/vnd.ibm.kms.kmip_client_certificate+json"
	ListCollectionMetadataCollectionMetadata_CollectionType_ApplicationVndIbmKmsKmipObjectJSON            = "application/vnd.ibm.kms.kmip_object+json"
	ListCollectionMetadataCollectionMetadata_CollectionType_ApplicationVndIbmKmsPolicyJSON                = "application/vnd.ibm.kms.policy+json"
	ListCollectionMetadataCollectionMetadata_CollectionType_ApplicationVndIbmKmsRegistrationInputJSON     = "application/vnd.ibm.kms.registration_input+json"
	ListCollectionMetadataCollectionMetadata_CollectionType_ApplicationVndIbmKmsRegistrationJSON          = "application/vnd.ibm.kms.registration+json"
	ListCollectionMetadataCollectionMetadata_CollectionType_ApplicationVndIbmKmsResourceCrnJSON           = "application/vnd.ibm.kms.resource_crn+json"
)

func (*ListCollectionMetadataCollectionMetadata) isaListCollectionMetadata() bool {
	return true
}

// UnmarshalListCollectionMetadataCollectionMetadata unmarshals an instance of ListCollectionMetadataCollectionMetadata from the specified map of raw messages.
func UnmarshalListCollectionMetadataCollectionMetadata(m map[string]json.RawMessage, result interface{}) (err error) {
	obj := new(ListCollectionMetadataCollectionMetadata)
	err = core.UnmarshalPrimitive(m, "collectionType", &obj.CollectionType)
	if err != nil {
		err = core.SDKErrorf(err, "", "collectionType-error", common.GetComponentInfo())
		return
	}
	err = core.UnmarshalPrimitive(m, "collectionTotal", &obj.CollectionTotal)
	if err != nil {
		err = core.SDKErrorf(err, "", "collectionTotal-error", common.GetComponentInfo())
		return
	}
	reflect.ValueOf(result).Elem().Set(reflect.ValueOf(obj))
	return
}

// ListCollectionMetadataCollectionMetadataWithTotalCount : The metadata that describes the resource array.
// This model "extends" ListCollectionMetadata
type ListCollectionMetadataCollectionMetadataWithTotalCount struct {
	// The type of resources in the resource array.
	CollectionType *string `json:"collectionType" validate:"required"`

	// The number of elements in the resource array.
	CollectionTotal *int64 `json:"collectionTotal" validate:"required"`

	// The total number of elements that match the request, disregarding limit and offset.
	TotalCount *int64 `json:"totalCount,omitempty"`
}

// Constants associated with the ListCollectionMetadataCollectionMetadataWithTotalCount.CollectionType property.
// The type of resources in the resource array.
const (
	ListCollectionMetadataCollectionMetadataWithTotalCount_CollectionType_ApplicationVndIbmKmsAliasJSON                 = "application/vnd.ibm.kms.alias+json"
	ListCollectionMetadataCollectionMetadataWithTotalCount_CollectionType_ApplicationVndIbmKmsAllowedIpMetadataJSON     = "application/vnd.ibm.kms.allowed_ip_metadata+json"
	ListCollectionMetadataCollectionMetadataWithTotalCount_CollectionType_ApplicationVndIbmKmsCrnJSON                   = "application/vnd.ibm.kms.crn+json"
	ListCollectionMetadataCollectionMetadataWithTotalCount_CollectionType_ApplicationVndIbmKmsErrorJSON                 = "application/vnd.ibm.kms.error+json"
	ListCollectionMetadataCollectionMetadataWithTotalCount_CollectionType_ApplicationVndIbmKmsEventAcknowledgeJSON      = "application/vnd.ibm.kms.event_acknowledge+json"
	ListCollectionMetadataCollectionMetadataWithTotalCount_CollectionType_ApplicationVndIbmKmsImportTokenJSON           = "application/vnd.ibm.kms.import_token+json"
	ListCollectionMetadataCollectionMetadataWithTotalCount_CollectionType_ApplicationVndIbmKmsKeyActionJSON             = "application/vnd.ibm.kms.key_action+json"
	ListCollectionMetadataCollectionMetadataWithTotalCount_CollectionType_ApplicationVndIbmKmsKeyJSON                   = "application/vnd.ibm.kms.key+json"
	ListCollectionMetadataCollectionMetadataWithTotalCount_CollectionType_ApplicationVndIbmKmsKeyRingJSON               = "application/vnd.ibm.kms.key_ring+json"
	ListCollectionMetadataCollectionMetadataWithTotalCount_CollectionType_ApplicationVndIbmKmsKmipAdapterJSON           = "application/vnd.ibm.kms.kmip_adapter+json"
	ListCollectionMetadataCollectionMetadataWithTotalCount_CollectionType_ApplicationVndIbmKmsKmipClientCertificateJSON = "application/vnd.ibm.kms.kmip_client_certificate+json"
	ListCollectionMetadataCollectionMetadataWithTotalCount_CollectionType_ApplicationVndIbmKmsKmipObjectJSON            = "application/vnd.ibm.kms.kmip_object+json"
	ListCollectionMetadataCollectionMetadataWithTotalCount_CollectionType_ApplicationVndIbmKmsPolicyJSON                = "application/vnd.ibm.kms.policy+json"
	ListCollectionMetadataCollectionMetadataWithTotalCount_CollectionType_ApplicationVndIbmKmsRegistrationInputJSON     = "application/vnd.ibm.kms.registration_input+json"
	ListCollectionMetadataCollectionMetadataWithTotalCount_CollectionType_ApplicationVndIbmKmsRegistrationJSON          = "application/vnd.ibm.kms.registration+json"
	ListCollectionMetadataCollectionMetadataWithTotalCount_CollectionType_ApplicationVndIbmKmsResourceCrnJSON           = "application/vnd.ibm.kms.resource_crn+json"
)

func (*ListCollectionMetadataCollectionMetadataWithTotalCount) isaListCollectionMetadata() bool {
	return true
}

// UnmarshalListCollectionMetadataCollectionMetadataWithTotalCount unmarshals an instance of ListCollectionMetadataCollectionMetadataWithTotalCount from the specified map of raw messages.
func UnmarshalListCollectionMetadataCollectionMetadataWithTotalCount(m map[string]json.RawMessage, result interface{}) (err error) {
	obj := new(ListCollectionMetadataCollectionMetadataWithTotalCount)
	err = core.UnmarshalPrimitive(m, "collectionType", &obj.CollectionType)
	if err != nil {
		err = core.SDKErrorf(err, "", "collectionType-error", common.GetComponentInfo())
		return
	}
	err = core.UnmarshalPrimitive(m, "collectionTotal", &obj.CollectionTotal)
	if err != nil {
		err = core.SDKErrorf(err, "", "collectionTotal-error", common.GetComponentInfo())
		return
	}
	err = core.UnmarshalPrimitive(m, "totalCount", &obj.TotalCount)
	if err != nil {
		err = core.SDKErrorf(err, "", "totalCount-error", common.GetComponentInfo())
		return
	}
	reflect.ValueOf(result).Elem().Set(reflect.ValueOf(obj))
	return
}

// SetInstancePoliciesOneOfSetInstancePolicyAllowedIP : Properties that are associated with setting an instance level allowed IP policy.
// This model "extends" SetInstancePoliciesOneOf
type SetInstancePoliciesOneOfSetInstancePolicyAllowedIP struct {
	// The metadata that describes the resource array.
	Metadata *CollectionMetadata `json:"metadata" validate:"required"`

	// A collection of resources.
	Resources []SetInstancePoliciesOneOfSetInstancePolicyAllowedIPResourcesItem `json:"resources" validate:"required"`
}

// NewSetInstancePoliciesOneOfSetInstancePolicyAllowedIP : Instantiate SetInstancePoliciesOneOfSetInstancePolicyAllowedIP (Generic Model Constructor)
func (*IbmKeyProtectApiV2) NewSetInstancePoliciesOneOfSetInstancePolicyAllowedIP(metadata *CollectionMetadata, resources []SetInstancePoliciesOneOfSetInstancePolicyAllowedIPResourcesItem) (_model *SetInstancePoliciesOneOfSetInstancePolicyAllowedIP, err error) {
	_model = &SetInstancePoliciesOneOfSetInstancePolicyAllowedIP{
		Metadata:  metadata,
		Resources: resources,
	}
	err = core.ValidateStruct(_model, "required parameters")
	if err != nil {
		err = core.SDKErrorf(err, "", "model-missing-required", common.GetComponentInfo())
	}
	return
}

func (*SetInstancePoliciesOneOfSetInstancePolicyAllowedIP) isaSetInstancePoliciesOneOf() bool {
	return true
}

// UnmarshalSetInstancePoliciesOneOfSetInstancePolicyAllowedIP unmarshals an instance of SetInstancePoliciesOneOfSetInstancePolicyAllowedIP from the specified map of raw messages.
func UnmarshalSetInstancePoliciesOneOfSetInstancePolicyAllowedIP(m map[string]json.RawMessage, result interface{}) (err error) {
	obj := new(SetInstancePoliciesOneOfSetInstancePolicyAllowedIP)
	err = core.UnmarshalModel(m, "metadata", &obj.Metadata, UnmarshalCollectionMetadata)
	if err != nil {
		err = core.SDKErrorf(err, "", "metadata-error", common.GetComponentInfo())
		return
	}
	err = core.UnmarshalModel(m, "resources", &obj.Resources, UnmarshalSetInstancePoliciesOneOfSetInstancePolicyAllowedIPResourcesItem)
	if err != nil {
		err = core.SDKErrorf(err, "", "resources-error", common.GetComponentInfo())
		return
	}
	reflect.ValueOf(result).Elem().Set(reflect.ValueOf(obj))
	return
}

// SetInstancePoliciesOneOfSetInstancePolicyAllowedNetwork : Properties that are associated with setting an instance level allowed network policy.
// This model "extends" SetInstancePoliciesOneOf
type SetInstancePoliciesOneOfSetInstancePolicyAllowedNetwork struct {
	// The metadata that describes the resource array.
	Metadata *CollectionMetadata `json:"metadata" validate:"required"`

	// A collection of resources.
	Resources []SetInstancePoliciesOneOfSetInstancePolicyAllowedNetworkResourcesItem `json:"resources" validate:"required"`
}

// NewSetInstancePoliciesOneOfSetInstancePolicyAllowedNetwork : Instantiate SetInstancePoliciesOneOfSetInstancePolicyAllowedNetwork (Generic Model Constructor)
func (*IbmKeyProtectApiV2) NewSetInstancePoliciesOneOfSetInstancePolicyAllowedNetwork(metadata *CollectionMetadata, resources []SetInstancePoliciesOneOfSetInstancePolicyAllowedNetworkResourcesItem) (_model *SetInstancePoliciesOneOfSetInstancePolicyAllowedNetwork, err error) {
	_model = &SetInstancePoliciesOneOfSetInstancePolicyAllowedNetwork{
		Metadata:  metadata,
		Resources: resources,
	}
	err = core.ValidateStruct(_model, "required parameters")
	if err != nil {
		err = core.SDKErrorf(err, "", "model-missing-required", common.GetComponentInfo())
	}
	return
}

func (*SetInstancePoliciesOneOfSetInstancePolicyAllowedNetwork) isaSetInstancePoliciesOneOf() bool {
	return true
}

// UnmarshalSetInstancePoliciesOneOfSetInstancePolicyAllowedNetwork unmarshals an instance of SetInstancePoliciesOneOfSetInstancePolicyAllowedNetwork from the specified map of raw messages.
func UnmarshalSetInstancePoliciesOneOfSetInstancePolicyAllowedNetwork(m map[string]json.RawMessage, result interface{}) (err error) {
	obj := new(SetInstancePoliciesOneOfSetInstancePolicyAllowedNetwork)
	err = core.UnmarshalModel(m, "metadata", &obj.Metadata, UnmarshalCollectionMetadata)
	if err != nil {
		err = core.SDKErrorf(err, "", "metadata-error", common.GetComponentInfo())
		return
	}
	err = core.UnmarshalModel(m, "resources", &obj.Resources, UnmarshalSetInstancePoliciesOneOfSetInstancePolicyAllowedNetworkResourcesItem)
	if err != nil {
		err = core.SDKErrorf(err, "", "resources-error", common.GetComponentInfo())
		return
	}
	reflect.ValueOf(result).Elem().Set(reflect.ValueOf(obj))
	return
}

// SetInstancePoliciesOneOfSetInstancePolicyDualAuthDelete : Properties that are associated with setting a dual authorization delete instance policy.
// This model "extends" SetInstancePoliciesOneOf
type SetInstancePoliciesOneOfSetInstancePolicyDualAuthDelete struct {
	// The metadata that describes the resource array.
	Metadata *CollectionMetadata `json:"metadata" validate:"required"`

	// A collection of resources.
	Resources []SetInstancePolicyDualAuthDeleteResourcesItem `json:"resources" validate:"required"`
}

// NewSetInstancePoliciesOneOfSetInstancePolicyDualAuthDelete : Instantiate SetInstancePoliciesOneOfSetInstancePolicyDualAuthDelete (Generic Model Constructor)
func (*IbmKeyProtectApiV2) NewSetInstancePoliciesOneOfSetInstancePolicyDualAuthDelete(metadata *CollectionMetadata, resources []SetInstancePolicyDualAuthDeleteResourcesItem) (_model *SetInstancePoliciesOneOfSetInstancePolicyDualAuthDelete, err error) {
	_model = &SetInstancePoliciesOneOfSetInstancePolicyDualAuthDelete{
		Metadata:  metadata,
		Resources: resources,
	}
	err = core.ValidateStruct(_model, "required parameters")
	if err != nil {
		err = core.SDKErrorf(err, "", "model-missing-required", common.GetComponentInfo())
	}
	return
}

func (*SetInstancePoliciesOneOfSetInstancePolicyDualAuthDelete) isaSetInstancePoliciesOneOf() bool {
	return true
}

// UnmarshalSetInstancePoliciesOneOfSetInstancePolicyDualAuthDelete unmarshals an instance of SetInstancePoliciesOneOfSetInstancePolicyDualAuthDelete from the specified map of raw messages.
func UnmarshalSetInstancePoliciesOneOfSetInstancePolicyDualAuthDelete(m map[string]json.RawMessage, result interface{}) (err error) {
	obj := new(SetInstancePoliciesOneOfSetInstancePolicyDualAuthDelete)
	err = core.UnmarshalModel(m, "metadata", &obj.Metadata, UnmarshalCollectionMetadata)
	if err != nil {
		err = core.SDKErrorf(err, "", "metadata-error", common.GetComponentInfo())
		return
	}
	err = core.UnmarshalModel(m, "resources", &obj.Resources, UnmarshalSetInstancePolicyDualAuthDeleteResourcesItem)
	if err != nil {
		err = core.SDKErrorf(err, "", "resources-error", common.GetComponentInfo())
		return
	}
	reflect.ValueOf(result).Elem().Set(reflect.ValueOf(obj))
	return
}

// SetInstancePoliciesOneOfSetInstancePolicyKeyCreateImportAccess : Properties that are associated with setting an instance level key create and import access policy.
// This model "extends" SetInstancePoliciesOneOf
type SetInstancePoliciesOneOfSetInstancePolicyKeyCreateImportAccess struct {
	// The metadata that describes the resource array.
	Metadata *CollectionMetadata `json:"metadata" validate:"required"`

	// A collection of resources.
	Resources []SetInstancePoliciesOneOfSetInstancePolicyKeyCreateImportAccessResourcesItem `json:"resources" validate:"required"`
}

// NewSetInstancePoliciesOneOfSetInstancePolicyKeyCreateImportAccess : Instantiate SetInstancePoliciesOneOfSetInstancePolicyKeyCreateImportAccess (Generic Model Constructor)
func (*IbmKeyProtectApiV2) NewSetInstancePoliciesOneOfSetInstancePolicyKeyCreateImportAccess(metadata *CollectionMetadata, resources []SetInstancePoliciesOneOfSetInstancePolicyKeyCreateImportAccessResourcesItem) (_model *SetInstancePoliciesOneOfSetInstancePolicyKeyCreateImportAccess, err error) {
	_model = &SetInstancePoliciesOneOfSetInstancePolicyKeyCreateImportAccess{
		Metadata:  metadata,
		Resources: resources,
	}
	err = core.ValidateStruct(_model, "required parameters")
	if err != nil {
		err = core.SDKErrorf(err, "", "model-missing-required", common.GetComponentInfo())
	}
	return
}

func (*SetInstancePoliciesOneOfSetInstancePolicyKeyCreateImportAccess) isaSetInstancePoliciesOneOf() bool {
	return true
}

// UnmarshalSetInstancePoliciesOneOfSetInstancePolicyKeyCreateImportAccess unmarshals an instance of SetInstancePoliciesOneOfSetInstancePolicyKeyCreateImportAccess from the specified map of raw messages.
func UnmarshalSetInstancePoliciesOneOfSetInstancePolicyKeyCreateImportAccess(m map[string]json.RawMessage, result interface{}) (err error) {
	obj := new(SetInstancePoliciesOneOfSetInstancePolicyKeyCreateImportAccess)
	err = core.UnmarshalModel(m, "metadata", &obj.Metadata, UnmarshalCollectionMetadata)
	if err != nil {
		err = core.SDKErrorf(err, "", "metadata-error", common.GetComponentInfo())
		return
	}
	err = core.UnmarshalModel(m, "resources", &obj.Resources, UnmarshalSetInstancePoliciesOneOfSetInstancePolicyKeyCreateImportAccessResourcesItem)
	if err != nil {
		err = core.SDKErrorf(err, "", "resources-error", common.GetComponentInfo())
		return
	}
	reflect.ValueOf(result).Elem().Set(reflect.ValueOf(obj))
	return
}

// SetInstancePoliciesOneOfSetInstancePolicyMetrics : Properties that are associated with setting a metrics instance policy.
// This model "extends" SetInstancePoliciesOneOf
type SetInstancePoliciesOneOfSetInstancePolicyMetrics struct {
	// The metadata that describes the resource array.
	Metadata *CollectionMetadata `json:"metadata" validate:"required"`

	// A collection of resources.
	Resources []SetInstancePoliciesOneOfSetInstancePolicyMetricsResourcesItem `json:"resources" validate:"required"`
}

// NewSetInstancePoliciesOneOfSetInstancePolicyMetrics : Instantiate SetInstancePoliciesOneOfSetInstancePolicyMetrics (Generic Model Constructor)
func (*IbmKeyProtectApiV2) NewSetInstancePoliciesOneOfSetInstancePolicyMetrics(metadata *CollectionMetadata, resources []SetInstancePoliciesOneOfSetInstancePolicyMetricsResourcesItem) (_model *SetInstancePoliciesOneOfSetInstancePolicyMetrics, err error) {
	_model = &SetInstancePoliciesOneOfSetInstancePolicyMetrics{
		Metadata:  metadata,
		Resources: resources,
	}
	err = core.ValidateStruct(_model, "required parameters")
	if err != nil {
		err = core.SDKErrorf(err, "", "model-missing-required", common.GetComponentInfo())
	}
	return
}

func (*SetInstancePoliciesOneOfSetInstancePolicyMetrics) isaSetInstancePoliciesOneOf() bool {
	return true
}

// UnmarshalSetInstancePoliciesOneOfSetInstancePolicyMetrics unmarshals an instance of SetInstancePoliciesOneOfSetInstancePolicyMetrics from the specified map of raw messages.
func UnmarshalSetInstancePoliciesOneOfSetInstancePolicyMetrics(m map[string]json.RawMessage, result interface{}) (err error) {
	obj := new(SetInstancePoliciesOneOfSetInstancePolicyMetrics)
	err = core.UnmarshalModel(m, "metadata", &obj.Metadata, UnmarshalCollectionMetadata)
	if err != nil {
		err = core.SDKErrorf(err, "", "metadata-error", common.GetComponentInfo())
		return
	}
	err = core.UnmarshalModel(m, "resources", &obj.Resources, UnmarshalSetInstancePoliciesOneOfSetInstancePolicyMetricsResourcesItem)
	if err != nil {
		err = core.SDKErrorf(err, "", "resources-error", common.GetComponentInfo())
		return
	}
	reflect.ValueOf(result).Elem().Set(reflect.ValueOf(obj))
	return
}

// SetInstancePoliciesOneOfSetInstancePolicyRotation : Properties that are associated with setting an instance level rotation policy.
// This model "extends" SetInstancePoliciesOneOf
type SetInstancePoliciesOneOfSetInstancePolicyRotation struct {
	// The metadata that describes the resource array.
	Metadata *CollectionMetadata `json:"metadata" validate:"required"`

	// A collection of resources.
	Resources []SetInstancePoliciesOneOfSetInstancePolicyRotationResourcesItem `json:"resources" validate:"required"`
}

// NewSetInstancePoliciesOneOfSetInstancePolicyRotation : Instantiate SetInstancePoliciesOneOfSetInstancePolicyRotation (Generic Model Constructor)
func (*IbmKeyProtectApiV2) NewSetInstancePoliciesOneOfSetInstancePolicyRotation(metadata *CollectionMetadata, resources []SetInstancePoliciesOneOfSetInstancePolicyRotationResourcesItem) (_model *SetInstancePoliciesOneOfSetInstancePolicyRotation, err error) {
	_model = &SetInstancePoliciesOneOfSetInstancePolicyRotation{
		Metadata:  metadata,
		Resources: resources,
	}
	err = core.ValidateStruct(_model, "required parameters")
	if err != nil {
		err = core.SDKErrorf(err, "", "model-missing-required", common.GetComponentInfo())
	}
	return
}

func (*SetInstancePoliciesOneOfSetInstancePolicyRotation) isaSetInstancePoliciesOneOf() bool {
	return true
}

// UnmarshalSetInstancePoliciesOneOfSetInstancePolicyRotation unmarshals an instance of SetInstancePoliciesOneOfSetInstancePolicyRotation from the specified map of raw messages.
func UnmarshalSetInstancePoliciesOneOfSetInstancePolicyRotation(m map[string]json.RawMessage, result interface{}) (err error) {
	obj := new(SetInstancePoliciesOneOfSetInstancePolicyRotation)
	err = core.UnmarshalModel(m, "metadata", &obj.Metadata, UnmarshalCollectionMetadata)
	if err != nil {
		err = core.SDKErrorf(err, "", "metadata-error", common.GetComponentInfo())
		return
	}
	err = core.UnmarshalModel(m, "resources", &obj.Resources, UnmarshalSetInstancePoliciesOneOfSetInstancePolicyRotationResourcesItem)
	if err != nil {
		err = core.SDKErrorf(err, "", "resources-error", common.GetComponentInfo())
		return
	}
	reflect.ValueOf(result).Elem().Set(reflect.ValueOf(obj))
	return
}

// SetInstancePoliciesOneOfSetMultipleInstancePolicies : Properties that are associated with setting any type of instance level policy.
// This model "extends" SetInstancePoliciesOneOf
type SetInstancePoliciesOneOfSetMultipleInstancePolicies struct {
	// The metadata that describes the resource array.
	Metadata *CollectionMetadata `json:"metadata" validate:"required"`

	// A collection of resources.
	Resources []SetMultipleInstancePoliciesResourcesItem `json:"resources" validate:"required"`
}

// NewSetInstancePoliciesOneOfSetMultipleInstancePolicies : Instantiate SetInstancePoliciesOneOfSetMultipleInstancePolicies (Generic Model Constructor)
func (*IbmKeyProtectApiV2) NewSetInstancePoliciesOneOfSetMultipleInstancePolicies(metadata *CollectionMetadata, resources []SetMultipleInstancePoliciesResourcesItem) (_model *SetInstancePoliciesOneOfSetMultipleInstancePolicies, err error) {
	_model = &SetInstancePoliciesOneOfSetMultipleInstancePolicies{
		Metadata:  metadata,
		Resources: resources,
	}
	err = core.ValidateStruct(_model, "required parameters")
	if err != nil {
		err = core.SDKErrorf(err, "", "model-missing-required", common.GetComponentInfo())
	}
	return
}

func (*SetInstancePoliciesOneOfSetMultipleInstancePolicies) isaSetInstancePoliciesOneOf() bool {
	return true
}

// UnmarshalSetInstancePoliciesOneOfSetMultipleInstancePolicies unmarshals an instance of SetInstancePoliciesOneOfSetMultipleInstancePolicies from the specified map of raw messages.
func UnmarshalSetInstancePoliciesOneOfSetMultipleInstancePolicies(m map[string]json.RawMessage, result interface{}) (err error) {
	obj := new(SetInstancePoliciesOneOfSetMultipleInstancePolicies)
	err = core.UnmarshalModel(m, "metadata", &obj.Metadata, UnmarshalCollectionMetadata)
	if err != nil {
		err = core.SDKErrorf(err, "", "metadata-error", common.GetComponentInfo())
		return
	}
	err = core.UnmarshalModel(m, "resources", &obj.Resources, UnmarshalSetMultipleInstancePoliciesResourcesItem)
	if err != nil {
		err = core.SDKErrorf(err, "", "resources-error", common.GetComponentInfo())
		return
	}
	reflect.ValueOf(result).Elem().Set(reflect.ValueOf(obj))
	return
}

// SetKeyPoliciesOneOfSetKeyPolicyDualAuthDelete : Base schema for request of create/update of key level dual authorization delete policy.
// This model "extends" SetKeyPoliciesOneOf
type SetKeyPoliciesOneOfSetKeyPolicyDualAuthDelete struct {
	// The metadata that describes the resource array.
	Metadata *CollectionMetadata `json:"metadata" validate:"required"`

	// A collection of resources.
	Resources []KeyPolicyDualAuthDelete `json:"resources" validate:"required"`
}

// NewSetKeyPoliciesOneOfSetKeyPolicyDualAuthDelete : Instantiate SetKeyPoliciesOneOfSetKeyPolicyDualAuthDelete (Generic Model Constructor)
func (*IbmKeyProtectApiV2) NewSetKeyPoliciesOneOfSetKeyPolicyDualAuthDelete(metadata *CollectionMetadata, resources []KeyPolicyDualAuthDelete) (_model *SetKeyPoliciesOneOfSetKeyPolicyDualAuthDelete, err error) {
	_model = &SetKeyPoliciesOneOfSetKeyPolicyDualAuthDelete{
		Metadata:  metadata,
		Resources: resources,
	}
	err = core.ValidateStruct(_model, "required parameters")
	if err != nil {
		err = core.SDKErrorf(err, "", "model-missing-required", common.GetComponentInfo())
	}
	return
}

func (*SetKeyPoliciesOneOfSetKeyPolicyDualAuthDelete) isaSetKeyPoliciesOneOf() bool {
	return true
}

// UnmarshalSetKeyPoliciesOneOfSetKeyPolicyDualAuthDelete unmarshals an instance of SetKeyPoliciesOneOfSetKeyPolicyDualAuthDelete from the specified map of raw messages.
func UnmarshalSetKeyPoliciesOneOfSetKeyPolicyDualAuthDelete(m map[string]json.RawMessage, result interface{}) (err error) {
	obj := new(SetKeyPoliciesOneOfSetKeyPolicyDualAuthDelete)
	err = core.UnmarshalModel(m, "metadata", &obj.Metadata, UnmarshalCollectionMetadata)
	if err != nil {
		err = core.SDKErrorf(err, "", "metadata-error", common.GetComponentInfo())
		return
	}
	err = core.UnmarshalModel(m, "resources", &obj.Resources, UnmarshalKeyPolicyDualAuthDelete)
	if err != nil {
		err = core.SDKErrorf(err, "", "resources-error", common.GetComponentInfo())
		return
	}
	reflect.ValueOf(result).Elem().Set(reflect.ValueOf(obj))
	return
}

// SetKeyPoliciesOneOfSetKeyPolicyRotation : Base schema for request of create/update of key level rotation policy.
// This model "extends" SetKeyPoliciesOneOf
type SetKeyPoliciesOneOfSetKeyPolicyRotation struct {
	// The metadata that describes the resource array.
	Metadata *CollectionMetadata `json:"metadata" validate:"required"`

	// A collection of resources.
	Resources []KeyPolicyRotation `json:"resources" validate:"required"`
}

// NewSetKeyPoliciesOneOfSetKeyPolicyRotation : Instantiate SetKeyPoliciesOneOfSetKeyPolicyRotation (Generic Model Constructor)
func (*IbmKeyProtectApiV2) NewSetKeyPoliciesOneOfSetKeyPolicyRotation(metadata *CollectionMetadata, resources []KeyPolicyRotation) (_model *SetKeyPoliciesOneOfSetKeyPolicyRotation, err error) {
	_model = &SetKeyPoliciesOneOfSetKeyPolicyRotation{
		Metadata:  metadata,
		Resources: resources,
	}
	err = core.ValidateStruct(_model, "required parameters")
	if err != nil {
		err = core.SDKErrorf(err, "", "model-missing-required", common.GetComponentInfo())
	}
	return
}

func (*SetKeyPoliciesOneOfSetKeyPolicyRotation) isaSetKeyPoliciesOneOf() bool {
	return true
}

// UnmarshalSetKeyPoliciesOneOfSetKeyPolicyRotation unmarshals an instance of SetKeyPoliciesOneOfSetKeyPolicyRotation from the specified map of raw messages.
func UnmarshalSetKeyPoliciesOneOfSetKeyPolicyRotation(m map[string]json.RawMessage, result interface{}) (err error) {
	obj := new(SetKeyPoliciesOneOfSetKeyPolicyRotation)
	err = core.UnmarshalModel(m, "metadata", &obj.Metadata, UnmarshalCollectionMetadata)
	if err != nil {
		err = core.SDKErrorf(err, "", "metadata-error", common.GetComponentInfo())
		return
	}
	err = core.UnmarshalModel(m, "resources", &obj.Resources, UnmarshalKeyPolicyRotation)
	if err != nil {
		err = core.SDKErrorf(err, "", "resources-error", common.GetComponentInfo())
		return
	}
	reflect.ValueOf(result).Elem().Set(reflect.ValueOf(obj))
	return
}

// SetKeyPoliciesOneOfSetMultipleKeyPolicies : Properties that are associated with key.
// This model "extends" SetKeyPoliciesOneOf
type SetKeyPoliciesOneOfSetMultipleKeyPolicies struct {
	// The metadata that describes the resource array.
	Metadata *CollectionMetadata `json:"metadata" validate:"required"`

	// A collection of resources.
	Resources []SetMultipleKeyPoliciesResource `json:"resources" validate:"required"`
}

// NewSetKeyPoliciesOneOfSetMultipleKeyPolicies : Instantiate SetKeyPoliciesOneOfSetMultipleKeyPolicies (Generic Model Constructor)
func (*IbmKeyProtectApiV2) NewSetKeyPoliciesOneOfSetMultipleKeyPolicies(metadata *CollectionMetadata, resources []SetMultipleKeyPoliciesResource) (_model *SetKeyPoliciesOneOfSetMultipleKeyPolicies, err error) {
	_model = &SetKeyPoliciesOneOfSetMultipleKeyPolicies{
		Metadata:  metadata,
		Resources: resources,
	}
	err = core.ValidateStruct(_model, "required parameters")
	if err != nil {
		err = core.SDKErrorf(err, "", "model-missing-required", common.GetComponentInfo())
	}
	return
}

func (*SetKeyPoliciesOneOfSetMultipleKeyPolicies) isaSetKeyPoliciesOneOf() bool {
	return true
}

// UnmarshalSetKeyPoliciesOneOfSetMultipleKeyPolicies unmarshals an instance of SetKeyPoliciesOneOfSetMultipleKeyPolicies from the specified map of raw messages.
func UnmarshalSetKeyPoliciesOneOfSetMultipleKeyPolicies(m map[string]json.RawMessage, result interface{}) (err error) {
	obj := new(SetKeyPoliciesOneOfSetMultipleKeyPolicies)
	err = core.UnmarshalModel(m, "metadata", &obj.Metadata, UnmarshalCollectionMetadata)
	if err != nil {
		err = core.SDKErrorf(err, "", "metadata-error", common.GetComponentInfo())
		return
	}
	err = core.UnmarshalModel(m, "resources", &obj.Resources, UnmarshalSetMultipleKeyPoliciesResource)
	if err != nil {
		err = core.SDKErrorf(err, "", "resources-error", common.GetComponentInfo())
		return
	}
	reflect.ValueOf(result).Elem().Set(reflect.ValueOf(obj))
	return
}
