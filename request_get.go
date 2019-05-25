package swarmdkg

import (
	"fmt"
	"io/ioutil"
	"net/http"
)

// params[0] - resourceHash
// params[1] - additional base url (feed, raw, etc)
// params[2..] - additional query parameters in form "key=value"
func GetRequestBZZ(url string, params ...string) ([]byte, int, error) {
	urlBase := "bzz"
	if len(params) > 1 {
		urlBase = urlBase + "-" + params[1]
	}

	testBzzUrl := fmt.Sprintf("%s/%s:", url, urlBase)
	if len(params) > 0 {
		if params[0] != "" {
			resourceHash := params[0]
			testBzzUrl = fmt.Sprintf("%s/%s:/%s", url, urlBase, resourceHash)
		}

		for i := 2; i < len(params); i++ {
			if i == 2 {
				testBzzUrl = fmt.Sprintf("%s?%s", testBzzUrl, params[i])
			} else {
				testBzzUrl = fmt.Sprintf("%s&%s", testBzzUrl, params[i])
			}
		}
	}

	resp, err := http.Get(testBzzUrl)
	if err != nil {
		return nil, resp.StatusCode, err
	}
	defer resp.Body.Close()

	res, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return nil, resp.StatusCode, err
	}

	return res, resp.StatusCode, nil
}
