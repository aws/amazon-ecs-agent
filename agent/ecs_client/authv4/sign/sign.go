// Copyright 2014-2015 Amazon.com, Inc. or its affiliates. All Rights Reserved.
//
// Licensed under the Apache License, Version 2.0 (the "License"). You may
// not use this file except in compliance with the License. A copy of the
// License is located at
//
//	http://aws.amazon.com/apache2.0/
//
// or in the "license" file accompanying this file. This file is distributed
// on an "AS IS" BASIS, WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either
// express or implied. See the License for the specific language governing
// permissions and limitations under the License.

package sign

import (
	"bytes"
	"fmt"
	"io"
	"io/ioutil"
	"sort"
	"strings"
	"sync"
	"time"

	"path/filepath"

	"encoding/hex"
	"errors"

	"net/http"
	"net/url"

	"github.com/aws/amazon-ecs-agent/agent/ecs_client/authv4/credentials"
	"github.com/aws/amazon-ecs-agent/agent/ecs_client/authv4/signable"
	"crypto/hmac"
	"crypto/sha256"
)

type Signable signable.Signable

type SigningDay string

// Return date as formated string and day string
func NewSigningDay(date time.Time) (string, SigningDay) {
	day := date.UTC().Format(iso8601BasicFmt)
	return day, SigningDay(day[:8])
}

type SigningKey []byte

// Utility to calculate a signing key
func GetSigningKey(day SigningDay, secretKey, region, service string) SigningKey {
	awskey := "AWS4" + secretKey
	hashcode := hmac.New(sha256.New, []byte(awskey))
	hashcode.Write([]byte(day))
	kdate := hashcode.Sum(nil)

	hashcode = hmac.New(sha256.New, kdate)
	hashcode.Write([]byte(region))
	kregion := hashcode.Sum(nil)

	hashcode = hmac.New(sha256.New, kregion)
	hashcode.Write([]byte(service))
	kservice := hashcode.Sum(nil)

	hashcode = hmac.New(sha256.New, kservice)
	hashcode.Write([]byte("aws4_request"))
	ksigned := hashcode.Sum(nil)

	return ksigned
}

// map date string to signing key for that day
type signingKeyCache map[SigningDay]SigningKey

// returns dedup'd sorted headers + Host added if needed
func normalizeHeaders(headers []string) (string, []string) {

	// dedup headers
	headermap := map[string]bool{"host": true} // host is always required ... add the rest
	for _, h := range headers {
		headermap[strings.ToLower(h)] = true
	}
	var headers_sorted []string
	for key, _ := range headermap {
		headers_sorted = append(headers_sorted, key)
	}
	sort.Strings(headers_sorted)

	return strings.Join(headers_sorted, ";"), headers_sorted
}

// Object to sign Signables with AuthV4.
type Signer struct {
	sync.Mutex // to protect internal volatile structures

	region             string                            // the aws region (e.g., us-east-1) NOTE: must agree with server!
	service            string                            // the name of the service (e.g., redshift) NOTE: must agree with server!
	credentialProvider credentials.AWSCredentialProvider // CredentialProvider (may be refreshable, e.g. security tokens)
	credentials        *credentials.AWSCredentials       // Credentials produced from the above credential provider.

	sortedHeaders    []string // sorted list of headers to sign
	canonicalHeaders string   // sorted and ; delimited

	signingKeys signingKeyCache
}

// NewSigner creates a signer for the given region and service. It signs with
// the given credentialprovider and signs the headers "Host" plus whatever's in
// extraHeaders. It will add  the date and security token headers
func NewSigner(region, service string, credentialProvider credentials.AWSCredentialProvider, extraHeaders []string) *Signer {

	headers_listed, headers_sorted := normalizeHeaders(extraHeaders)

	currentCredentials, _ := credentialProvider.Credentials()
	return &Signer{
		Mutex:              sync.Mutex{},
		region:             strings.ToLower(region),
		service:            strings.ToLower(service),
		credentialProvider: credentialProvider,
		credentials:        currentCredentials,
		sortedHeaders:      headers_sorted,
		canonicalHeaders:   headers_listed,
		signingKeys:        make(signingKeyCache, 1),
	}
}

func (signer *Signer) GetSigningKey(today SigningDay) SigningKey {
	signer.Lock()
	defer signer.Unlock()

	ksigned, present := signer.signingKeys[today]

	if !present {
		ksigned = GetSigningKey(today, signer.credentials.SecretKey, signer.region, signer.service)
		signer.signingKeys[today] = ksigned
	}

	return ksigned
}

func normalizePath(p string) string {
	if strings.Contains(p, "./") || strings.Contains(p, "..") {
		p, _ = filepath.Abs(p)
	}

	if strings.Contains(p, "//") {
		p = strings.Replace(p, "//", "/", -1)
	}

	u := url.URL{Path: p}

	return u.RequestURI()
}

func sha256Digest(b []byte) string {
	hash := sha256.Sum256(b)
	return hex.EncodeToString(hash[:])
}

func canonicalizeQuery(signable Signable) string {
	// arrange query parameters for signing
	canonical_query := ""
	u := signable.ReqURL()
	if u.RawQuery != "" {
		// sort the queries, and for repeated params, sort the values
		values := u.Query()
		for _, value := range values {
			sort.Strings(value) // ensure values are sorted
		}
		canonical_query = values.Encode()
	}

	return canonical_query
}

func canonicalizeHeaders(sortedHeaders []string, signable Signable) []string {
	// arrange headers for signing
	// Host is required; always set it just in case
	signable.SetHeader("host", signable.GetHost())

	var canonical_headers []string
	for _, h := range sortedHeaders {
		values, present := signable.GetHeader(http.CanonicalHeaderKey(h))
		if present {
			// multiple values must be sorted
			sort.Strings(values)
			canonical_headers = append(canonical_headers, h+":"+strings.Join(values, ","))
		}
	}
	return canonical_headers
}

// return canonical_request, string to sign, and payload_signature
func getStringToSign(nowDate string, nowDay SigningDay,
	region, service, accessKey string,
	canonicalHeaders string, sortedHeaders []string,
	signable Signable) (string, string, string, error) {

	// hash payload (if we have a ReadSeeker, use it)
	var payload_signature string

	reqBody := signable.ReqBody()

	if reqBody == nil {

		payload_signature = sha256Digest(nil)

	} else {
		// see if the reader can Seek() (e.g., a file), else we need to copy into memory
		// avoids reading request data ALL into ram at once (we hope)
		if seeker, ok := reqBody.(io.ReadSeeker); ok {

			hasher := sha256.New()
			_, err := io.Copy(hasher, seeker)
			if err != nil {
				return "", "", "", err
			}
			_, err = seeker.Seek(0, 0)

			if err != nil {
				return "", "", "", err
			}

			hash := hasher.Sum(nil)
			payload_signature = hex.EncodeToString(hash[:])
		} else {
			payload, _ := ioutil.ReadAll(reqBody)

			// Reset the request body back to the beginning by recreating it
			body := ioutil.NopCloser(bytes.NewReader(payload))
			signable.SetReqBody(body)
			payload_signature = sha256Digest(payload)
		}

	}

	canonical_query := canonicalizeQuery(signable)

	canonical_headers := canonicalizeHeaders(sortedHeaders, signable)

	canonical_request := (signable.ReqMethod() + "\n" +
		normalizePath(signable.ReqURL().Path) + "\n" +
		canonical_query + "\n" +
		strings.Join(canonical_headers, "\n") + "\n\n" +
		canonicalHeaders + "\n" +
		payload_signature)

	canonical_request_signature := sha256Digest([]byte(canonical_request))

	str2sign := fmt.Sprintf("AWS4-HMAC-SHA256\n%s\n%s/%s/%s/aws4_request\n%s",
		nowDate, nowDay, region, service, canonical_request_signature)

	return canonical_request, str2sign, payload_signature, nil
}

// core signing routine: return canonical_request, final "Authorization" string and payload signature
func doSign(nowDate string, nowDay SigningDay,
	region, service, accessKey string,
	canonicalHeaders string, sortedHeaders []string,
	signingKey SigningKey,
	signable Signable) (string, string, string, error) {

	canonical_request, str2sign, payload_signature, err := getStringToSign(nowDate, nowDay, region, service, accessKey, canonicalHeaders, sortedHeaders, signable)
	if err != nil {
		return "", "", "", err
	}

	hashcode := hmac.New(sha256.New, signingKey)
	hashcode.Write([]byte(str2sign))
	signature := hex.EncodeToString(hashcode.Sum(nil))

	credential := fmt.Sprintf("%s/%s/%s/%s/aws4_request", accessKey, nowDay, region, service)

	authz := ("AWS4-HMAC-SHA256 Credential=" + credential +
		", SignedHeaders=" + canonicalHeaders +
		", Signature=" + signature)

	return canonical_request, authz, payload_signature, nil
}

// Useful for tests or other code that wants the SigningKey
func (signer *Signer) SignDetails(now time.Time, signable Signable) (SigningKey, string, error) {

	// format dates
	nowDate, nowDay := NewSigningDay(now)

	signingkey := signer.GetSigningKey(nowDay)

	// set date so we can sign it if directed to in extraHeaders
	signable.SetHeader("X-Amz-Date", nowDate)

	// Also set token if it's there so it can be signed
	if signer.credentials.Token != "" {
		signable.SetHeader("X-Amz-Security-Token", signer.credentials.Token)
	}

	_, authz, payload_signature, err := doSign(nowDate, nowDay,
		signer.region, signer.service, signer.credentials.AccessKey,
		signer.canonicalHeaders, signer.sortedHeaders,
		signingkey, signable)

	if err != nil {
		return signingkey, authz, err
	}

	// set headers with signing info
	signable.SetHeader("Authorization", authz)
	signable.SetHeader("X-Amz-Content-SHA256", payload_signature)

	return signingkey, authz, err
}

// RefreshCredentials checks if the AWSCredentialProvider needs to refresh
// credentials and does so as needed. The refreshed credentials are stored in
// the signer.
// We explicitly manage the credential refreshing because we don't want them to refresh
// without us knowing about it so we can always invalidate the cache
func (signer *Signer) RefreshCredentials() error {
	if refreshable, ok := signer.credentialProvider.(credentials.RefreshableAWSCredentialProvider); ok {
		if refreshable.NeedsRefresh() {
			// Lock it and invalidate the cache
			signer.Lock()
			defer signer.Unlock()
			refreshable.Refresh()

			// The signatures its cached so far are for expired creds; invalidate
			signer.signingKeys = make(signingKeyCache, 1)

			newCreds, err := refreshable.Credentials()
			if err == nil {
				signer.credentials = newCreds
			}
			return err
		}
	}
	return nil
}

// Sign request with AWS AuthV4 headers.
func (signer *Signer) Sign(signable Signable) error {
	if err := signer.RefreshCredentials(); err != nil {
		return err
	}
	if signer.credentials == nil {
		return errors.New("No credentials available to sign request")
	}

	_, _, err := signer.SignDetails(time.Now(), signable)
	return err
}
