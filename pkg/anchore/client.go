package anchore

import (
	"bytes"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"reflect"
	"strings"
	"time"

	"k8s.io/klog"
)

func NewClient(baseUrl, secureToken string) (*AnchoreClient, error) {
	client := AnchoreClient{
		baseUrl:     baseUrl,
		secureToken: secureToken,
	}

	return &client, nil
}

var (
	transCfg = &http.Transport{
		TLSClientConfig: &tls.Config{InsecureSkipVerify: true}, // ignore expired SSL certificates
	}

	client = &http.Client{
		Transport: transCfg,
	}
)

const errNotFound = "response from Anchore: 404"

func (c *AnchoreClient) anchoreRequest(path string, bodyParams map[string]string, method string) ([]byte, error) {
	fullURL := c.baseUrl + path

	bodyParamJson, err := json.Marshal(bodyParams)
	req, err := http.NewRequest(method, fullURL, bytes.NewBuffer(bodyParamJson))
	if err != nil {
		klog.Fatal(err)
	}

	req.SetBasicAuth(c.secureToken, "")
	klog.Infof("[Anchore] Sending request to %s, with params %s", fullURL, bodyParams)
	req.Header.Add("Content-Type", "application/json")

	resp, err := client.Do(req)

	if err != nil {
		return nil, fmt.Errorf("failed to complete request to Anchore: %v", err)
	}

	bodyText, err := ioutil.ReadAll(resp.Body)

	if err != nil {
		return nil, fmt.Errorf("failed to complete request to Anchore: %v", err)
	}
	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("response from Anchore: %d", resp.StatusCode)
	}
	return bodyText, nil
}

func (c *AnchoreClient) getReport(digest string, tag string) (*ScanReport, error) {
	path := fmt.Sprintf("/images/%s/check?tag=%s&history=false&detail=true", digest, tag)
	body, err := c.anchoreRequest(path, nil, "GET")

	if err != nil && err.Error() == errNotFound {
		// first time scanned image, return true
		klog.Warningf("[Anchore] image %s with tag %s has not been scanned.", digest, tag)
		//TODO: Report why the image is rejected
		return nil, err
	}

	if err != nil {
		klog.Errorf("[Anchore] scan error %v", err)
		//TODO: Report why the image is rejected
		return nil, err
	}

	ret := string(body)
	ret = strings.Replace(ret, "\t", "  ", -1)

	klog.Infof("[Anchore] Anchore Response Body: %s", ret)

	var result ScanReports
	err = json.Unmarshal(body, &result)
	if err != nil {
		klog.Errorf("[Anchore] Body unmarshall error %v", err)
		//TODO: Report why the image is rejected
		return nil, err
	}

	if len(result) == 0 {
		klog.Errorf("[Anchore] Scan report list is empty")
		return nil, fmt.Errorf("Scan report list is empty")
	}

	if len(result) > 1 {
		klog.Errorf("[Anchore] Unexpected scan report: multiple entries")
		return nil, fmt.Errorf("Unexpected scan report: multiple entries")
	}

	if _, ok := result[0][digest]; !ok {
		klog.Errorf("[Anchore] Digest in the scan report does not match")
		return nil, fmt.Errorf("Digest in the scan report does not match")
	}

	fullTag := reflect.ValueOf(result[0][digest]).MapKeys()[0].String()

	return &result[0][digest][fullTag][0], nil
}

func (c *AnchoreClient) getStatus(digest string, tag string) (bool, error) {
	result, err := c.getReport(digest, tag)

	if err != nil {
		return false, err
	}

	// foundStatus := findStatus(result)

	if strings.ToLower(result.Status) == "pass" {
		return true, nil
	} else {
		return false, fmt.Errorf("Scan result is FAILED")
	}
}

func (c *AnchoreClient) getDigest(imageRef string) (string, error) {
	// Tag or repo??
	params := map[string]string{
		"tag":     imageRef,
		"history": "true",
	}

	body, err := c.anchoreRequest("/images", params, "GET")
	if err != nil {
		klog.Errorf("[Anchore] %v", err)
		return "", err
	}

	var images []Image
	err = json.Unmarshal(body, &images)

	if err != nil {
		return "", fmt.Errorf("failed to unmarshal JSON from response: %v", err)
	}

	return images[0].ImageDigest, nil
}

func (c *AnchoreClient) addImage(image string) error {
	params := map[string]string{"tag": image}
	_, err := c.anchoreRequest("/images", params, "POST")
	if err != nil {
		return err
	}

	klog.Infof("[Anchore] Added image to Anchore Engine: %s", image)
	return nil
}

func (c *AnchoreClient) GetImageDigest(image string) (digest string, err error) {
	err = c.addImage(image)
	if err != nil {
		klog.Errorf("[Anchore] addImage error: %s", err)
		return
	}

	count := 0
	for {
		digest, err = c.getDigest(image)
		if err == nil {
			return
		}

		klog.Errorf("[Anchore] getDigest error: %s", err)
		if count >= 5 {
			return
		}

		time.Sleep(time.Second)
		count++
	}
}

func (c *AnchoreClient) CheckImage(image string) (bool, error) {
	digest, err := c.GetImageDigest(image)
	if err != nil {
		return false, fmt.Errorf("Unable to obtain image digest")
	}
	return c.getStatus(digest, image)
}

func (c *AnchoreClient) GetScanReport(image string) (*ScanReport, error) {
	digest, err := c.GetImageDigest(image)
	if err != nil {
		return nil, fmt.Errorf("Unable to obtain image digest")
	}
	return c.getReport(digest, image)
}
