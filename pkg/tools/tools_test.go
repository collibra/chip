package tools_test

import (
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/http/httptest"
	"net/url"
	"path"
)

type testServer map[string]http.Handler

func (s testServer) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	handler, ok := s[r.URL.Path]
	if !ok {
		http.Error(w, fmt.Sprintf("Handler for path: %s not found", r.URL.Path), http.StatusNotFound)
		return
	}

	handler.ServeHTTP(w, r)
}

type testClient struct {
	baseURL string
	next    http.RoundTripper
}

func (c *testClient) RoundTrip(request *http.Request) (*http.Response, error) {
	reqClone := request.Clone(request.Context())
	baseURL, err := url.Parse(c.baseURL)
	if err != nil {
		return nil, err
	}
	reqClone.URL.Scheme = baseURL.Scheme
	reqClone.URL.Host = baseURL.Host
	reqClone.URL.Path = path.Join(baseURL.Path, request.URL.Path)
	return c.next.RoundTrip(reqClone)
}

func newClient(server *httptest.Server) *http.Client {
	return &http.Client{Transport: &testClient{baseURL: server.URL, next: http.DefaultTransport}}
}

type Marshaller[Type any] interface {
	Marshall(v Type) ([]byte, error)
}

type Unmarshaller[Type any] interface {
	Unmarshal(data []byte) (Type, error)
}

type MarshallerFunc[Type any] func(v Type) ([]byte, error)

func (f MarshallerFunc[Type]) Marshall(v Type) ([]byte, error) {
	return f(v)
}

type UnmarshallerFunc[Type any] func([]byte) (Type, error)

func (f UnmarshallerFunc[Type]) Unmarshal(data []byte) (Type, error) {
	return f(data)
}

func JsonMarshaller[Type any](value Type) ([]byte, error) {
	return json.Marshal(value)
}

func JsonUnmarshaller[Type any](data []byte) (Type, error) {
	var value Type
	err := json.Unmarshal(data, &value)
	return value, err
}

func StringMarshaller(value string) ([]byte, error) {
	return []byte(value), nil
}

func StringUnmarshaller(data []byte) (string, error) {
	return string(data), nil
}

func HttpHandlerInOut[In, Out any](m Marshaller[Out], u Unmarshaller[In], handler func(r *http.Request, in In) Out) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		request, err := io.ReadAll(r.Body)
		if err != nil {
			http.Error(w, "Failed to read request: "+err.Error(), http.StatusBadRequest)
			return
		}
		in, err := u.Unmarshal(request)
		if err != nil {
			http.Error(w, "Failed to unmarshall request: "+err.Error(), http.StatusBadRequest)
			return
		}
		out := handler(r, in)
		response, err := m.Marshall(out)
		if err != nil {
			http.Error(w, "Failed to Marshall response: "+err.Error(), http.StatusInternalServerError)
			return
		}

		w.WriteHeader(http.StatusOK)
		_, err = w.Write(response)
		if err != nil {
			log.Printf("Failed to write response: %v", err)
		}
	})
}

func HttpHandlerOut[Out any](m Marshaller[Out], handler func(r *http.Request) Out) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		out := handler(r)
		response, err := m.Marshall(out)
		if err != nil {
			http.Error(w, "Failed to Marshall response: "+err.Error(), http.StatusInternalServerError)
			return
		}

		w.WriteHeader(http.StatusOK)
		_, err = w.Write(response)
		if err != nil {
			log.Printf("Failed to write response: %v", err)
		}
	})
}

func JsonHandlerInOut[In, Out any](handler func(r *http.Request, in In) Out) http.Handler {
	return HttpHandlerInOut[In, Out](MarshallerFunc[Out](JsonMarshaller[Out]), UnmarshallerFunc[In](JsonUnmarshaller[In]), handler)
}

func JsonHandlerOut[Out any](handler func(r *http.Request) Out) http.Handler {
	return HttpHandlerOut[Out](MarshallerFunc[Out](JsonMarshaller[Out]), handler)
}

func StringHandlerInOut(handler func(r *http.Request, in string) string) http.Handler {
	return HttpHandlerInOut(MarshallerFunc[string](StringMarshaller), UnmarshallerFunc[string](StringUnmarshaller), handler)
}

func StringHandlerOut(handler func(r *http.Request) string) http.Handler {
	return HttpHandlerOut(MarshallerFunc[string](StringMarshaller), handler)
}
