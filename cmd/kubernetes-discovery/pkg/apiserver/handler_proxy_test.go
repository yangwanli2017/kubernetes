/*
Copyright 2016 The Kubernetes Authors.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package apiserver

import (
	"io/ioutil"
	"net/http"
	"net/http/httptest"
	"net/http/httputil"
	"reflect"
	"strings"
	"testing"

	"k8s.io/kubernetes/pkg/api"
	"k8s.io/kubernetes/pkg/auth/user"
	genericapirequest "k8s.io/kubernetes/pkg/genericapiserver/api/request"
	"k8s.io/kubernetes/pkg/util/sets"

	"k8s.io/kubernetes/cmd/kubernetes-discovery/pkg/apis/apiregistration"
)

type targetHTTPHandler struct {
	called  bool
	headers map[string][]string
	path    string
}

func (d *targetHTTPHandler) Reset() {
	d.path = ""
	d.called = false
	d.headers = nil
}

func (d *targetHTTPHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	d.path = r.URL.Path
	d.called = true
	d.headers = r.Header
	w.WriteHeader(http.StatusOK)
}

type fakeRequestContextMapper struct {
	user user.Info
}

func (m *fakeRequestContextMapper) Get(req *http.Request) (genericapirequest.Context, bool) {
	ctx := genericapirequest.NewContext()
	if m.user != nil {
		ctx = genericapirequest.WithUser(ctx, m.user)
	}

	resolver := &genericapirequest.RequestInfoFactory{
		APIPrefixes:          sets.NewString("api", "apis"),
		GrouplessAPIPrefixes: sets.NewString("api"),
	}
	info, err := resolver.NewRequestInfo(req)
	if err == nil {
		ctx = genericapirequest.WithRequestInfo(ctx, info)
	}

	return ctx, true
}

func (*fakeRequestContextMapper) Update(req *http.Request, context genericapirequest.Context) error {
	return nil
}

func TestProxyHandler(t *testing.T) {
	target := &targetHTTPHandler{}
	targetServer := httptest.NewTLSServer(target)
	defer targetServer.Close()

	handler := &proxyHandler{}

	server := httptest.NewServer(handler)
	defer server.Close()

	tests := map[string]struct {
		user       user.Info
		path       string
		apiService *apiregistration.APIService

		expectedStatusCode int
		expectedBody       string
		expectedCalled     bool
		expectedHeaders    map[string][]string
	}{
		"no target": {
			expectedStatusCode: http.StatusNotFound,
		},
		"no user": {
			apiService: &apiregistration.APIService{
				ObjectMeta: api.ObjectMeta{Name: "v1.foo"},
				Spec: apiregistration.APIServiceSpec{
					Group:   "foo",
					Version: "v1",
				},
			},
			expectedStatusCode: http.StatusInternalServerError,
			expectedBody:       "missing user",
		},
		"proxy with user": {
			user: &user.DefaultInfo{
				Name:   "username",
				Groups: []string{"one", "two"},
			},
			path: "/request/path",
			apiService: &apiregistration.APIService{
				ObjectMeta: api.ObjectMeta{Name: "v1.foo"},
				Spec: apiregistration.APIServiceSpec{
					Group:                 "foo",
					Version:               "v1",
					InsecureSkipTLSVerify: true,
				},
			},
			expectedStatusCode: http.StatusOK,
			expectedCalled:     true,
			expectedHeaders: map[string][]string{
				"X-Forwarded-Proto": {"https"},
				"X-Forwarded-Uri":   {"/request/path"},
				"X-Remote-User":     {"username"},
				"User-Agent":        {"Go-http-client/1.1"},
				"Accept-Encoding":   {"gzip"},
				"X-Remote-Group":    {"one", "two"},
			},
		},
		"fail on bad serving cert": {
			user: &user.DefaultInfo{
				Name:   "username",
				Groups: []string{"one", "two"},
			},
			path: "/request/path",
			apiService: &apiregistration.APIService{
				ObjectMeta: api.ObjectMeta{Name: "v1.foo"},
				Spec: apiregistration.APIServiceSpec{
					Group:   "foo",
					Version: "v1",
				},
			},
			expectedStatusCode: http.StatusServiceUnavailable,
		},
	}

	for name, tc := range tests {
		target.Reset()
		handler.contextMapper = &fakeRequestContextMapper{user: tc.user}
		handler.removeAPIService()
		if tc.apiService != nil {
			handler.updateAPIService(tc.apiService)
			handler.destinationHost = targetServer.Listener.Addr().String()
		}

		resp, err := http.Get(server.URL + tc.path)
		if err != nil {
			t.Errorf("%s: %v", name, err)
			continue
		}
		if e, a := tc.expectedStatusCode, resp.StatusCode; e != a {
			body, _ := httputil.DumpResponse(resp, true)
			t.Logf("%s: %v", name, string(body))
			t.Errorf("%s: expected %v, got %v", name, e, a)
			continue
		}
		bytes, err := ioutil.ReadAll(resp.Body)
		if err != nil {
			t.Errorf("%s: %v", name, err)
			continue
		}
		if !strings.Contains(string(bytes), tc.expectedBody) {
			t.Errorf("%s: expected %q, got %q", name, tc.expectedBody, string(bytes))
			continue
		}

		if e, a := tc.expectedCalled, target.called; e != a {
			t.Errorf("%s: expected %v, got %v", name, e, a)
			continue
		}
		// this varies every test
		delete(target.headers, "X-Forwarded-Host")
		if e, a := tc.expectedHeaders, target.headers; !reflect.DeepEqual(e, a) {
			t.Errorf("%s: expected %v, got %v", name, e, a)
			continue
		}
	}
}
