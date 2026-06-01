package tekton

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

const quayBaseUrl = "https://quay.io/api/v1"

type CosignResult struct {
	SignatureImageRef   string
	AttestationImageRef string
}

type Tag struct {
	Digest string `json:"manifest_digest"`
}

type TagResponse struct {
	Tags []Tag `json:"tags"`
}

func FindCosignResultsForImage(imageRef string) (*CosignResult, error) {
	var errMsg string
	imageInfo := strings.Split(imageRef, "@")
	if len(imageInfo) < 2 {
		return nil, fmt.Errorf("image reference %q does not contain a digest (expected format: repo@sha256:...)", imageRef)
	}
	imageRegistryName := strings.Split(imageInfo[0], "/")[0]
	imageRepoName := strings.Split(strings.TrimPrefix(imageInfo[0], fmt.Sprintf("%s/", imageRegistryName)), ":")[0]
	imageTagPrefix := strings.Replace(imageInfo[1], ":", "-", 1)

	results := CosignResult{}
	signatureTag, err := getImageInfoFromQuay(imageRepoName, imageTagPrefix+".sig")
	if err != nil {
		errMsg += fmt.Sprintf("error when getting signature tag: %+v\n", err)
	} else {
		results.SignatureImageRef = signatureTag
	}

	attestationTag, err := getImageInfoFromQuay(imageRepoName, imageTagPrefix+".att")
	if err != nil {
		errMsg += fmt.Sprintf("error when getting attestation tag: %+v\n", err)
	} else {
		results.AttestationImageRef = attestationTag
	}

	if len(errMsg) > 0 {
		return &results, fmt.Errorf("failed to find cosign results for image %s: %s", imageRef, errMsg)
	}
	return &results, nil
}

func (c CosignResult) IsPresent() bool {
	return c.SignatureImageRef != "" && c.AttestationImageRef != ""
}

func (c CosignResult) Missing(prefix string) string {
	ret := make([]string, 0, 2)
	if c.SignatureImageRef == "" {
		ret = append(ret, prefix+".sig")
	}
	if c.AttestationImageRef == "" {
		ret = append(ret, prefix+".att")
	}
	return strings.Join(ret, " and ")
}

var quayHTTPClient = &http.Client{Timeout: 30 * time.Second}

func getImageInfoFromQuay(imageRepo, imageTag string) (string, error) {
	res, err := quayHTTPClient.Get(fmt.Sprintf("%s/repository/%s/tag/?specificTag=%s", quayBaseUrl, imageRepo, imageTag))
	if err != nil {
		return "", fmt.Errorf("cannot get quay.io/%s:%s from registry: %+v", imageRepo, imageTag, err)
	}
	defer res.Body.Close()
	if res.StatusCode < 200 || res.StatusCode >= 300 {
		return "", fmt.Errorf("unexpected status %d for quay.io/%s:%s", res.StatusCode, imageRepo, imageTag)
	}
	body, err := io.ReadAll(res.Body)
	if err != nil {
		return "", fmt.Errorf("cannot read response for quay.io/%s:%s: %+v", imageRepo, imageTag, err)
	}

	tagResponse := &TagResponse{}
	if err = json.Unmarshal(body, tagResponse); err != nil {
		return "", fmt.Errorf("failed to unmarshal response for quay.io/%s:%s: %+v", imageRepo, imageTag, err)
	}

	if len(tagResponse.Tags) < 1 {
		return "", fmt.Errorf("cannot get manifest digest from quay.io/%s:%s, body: %s", imageRepo, imageTag, string(body))
	}

	return fmt.Sprintf("quay.io/%s@%s", imageRepo, tagResponse.Tags[0].Digest), nil
}
