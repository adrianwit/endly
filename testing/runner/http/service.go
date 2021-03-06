package http

import (
	"bytes"
	"encoding/base64"
	"fmt"
	"github.com/viant/endly"
	"github.com/viant/endly/testing/validator"
	"github.com/viant/endly/udf"
	"github.com/viant/endly/util"
	"github.com/viant/toolbox"
	"github.com/viant/toolbox/data"
	"io"
	"io/ioutil"
	"net/http"
	"strings"
	"time"
)

//ServiceID represents http runner service id.
const ServiceID = "http/runner"

//PreviousTripStateKey keys to store previous request details for multi trip HTTP Send request in context state
const PreviousTripStateKey = "previous"

type service struct {
	*endly.AbstractService
}

func (s *service) processResponse(context *endly.Context, sendRequest *SendRequest, sendHTTPRequest *Request, response *Response, httpResponse *http.Response, isBase64Encoded bool, extracted map[string]interface{}) (string, error) {
	response.Header = make(map[string][]string)
	copyHeaders(httpResponse.Header, response.Header)
	readBody(httpResponse, response, isBase64Encoded)
	if sendHTTPRequest.ResponseUdf != "" {
		transformed, err := udf.TransformWithUDF(context, sendHTTPRequest.ResponseUdf, sendHTTPRequest.URL, response.Body)
		if err != nil {
			return "", err
		}
		if toolbox.IsMap(transformed) {
			response.Body, _ = toolbox.AsJSONText(transformed)
		} else {
			response.Body = toolbox.AsString(transformed)
		}
	}

	var responseBody = replaceResponseBodyIfNeeded(sendHTTPRequest, response.Body)
	return responseBody, nil
}

func (s *service) sendRequest(context *endly.Context, client *http.Client, HTTPRequest *Request, sessionCookies *Cookies, sendGroupRequest *SendRequest, sendGroupResponse *SendResponse) error {
	var err error
	var state = context.State()
	cookies := state.GetMap("cookies")
	var reader io.Reader
	var isBase64Encoded = false
	HTTPRequest = HTTPRequest.Expand(context)
	var body []byte
	var ok bool
	if len(HTTPRequest.Body) > 0 {
		body = []byte(HTTPRequest.Body)
		if HTTPRequest.RequestUdf != "" {
			transformed, err := udf.TransformWithUDF(context, HTTPRequest.RequestUdf, HTTPRequest.URL, string(body))
			if err != nil {
				return err
			}
			if body, ok = transformed.([]byte); !ok {
				body = []byte(toolbox.AsString(transformed))
			}
		}
		isBase64Encoded = strings.HasPrefix(string(body), "base64:")
		body, err = util.FromPayload(string(body))
		if err != nil {
			return err
		}
		reader = bytes.NewReader(body)
	}

	httpRequest, err := http.NewRequest(strings.ToUpper(HTTPRequest.Method), HTTPRequest.URL, reader)
	if err != nil {
		return err
	}

	copyHeaders(HTTPRequest.Header, httpRequest.Header)
	sessionCookies.SetHeader(HTTPRequest.Header)
	HTTPRequest.Cookies.SetHeader(httpRequest.Header)

	response := &Response{}
	sendGroupResponse.Responses = append(sendGroupResponse.Responses, response)
	startEvent := s.Begin(context, HTTPRequest)
	repeater := HTTPRequest.Repeater.Init()

	var HTTPResponse *http.Response
	var responseBody string
	var bodyCache []byte
	var useCachedBody = repeater.Repeat > 1 && httpRequest.ContentLength > 0
	if useCachedBody {
		bodyCache, err = ioutil.ReadAll(httpRequest.Body)
		if err != nil {
			return err
		}
	}

	handler := func() (interface{}, error) {
		if useCachedBody {
			httpRequest.Body = ioutil.NopCloser(bytes.NewReader(bodyCache))
		}

		HTTPResponse, err = client.Do(httpRequest)
		if err != nil {
			return nil, err
		}
		responseBody, err = s.processResponse(context, sendGroupRequest, HTTPRequest, response, HTTPResponse, isBase64Encoded, sendGroupResponse.Data)
		return responseBody, err
	}

	err = repeater.Run(s.AbstractService, "HTTPRunner", context, handler, sendGroupResponse.Data)
	if err != nil {
		return err
	}
	var responseCookies Cookies = HTTPResponse.Cookies()
	response.Cookies = responseCookies.IndexByName()
	for k, cookie := range response.Cookies {
		cookies.Put(k, cookie.Value)
	}
	sessionCookies.AddCookies(responseCookies...)
	endEvent := s.End(context)(startEvent, response)

	var previous = state.GetMap(PreviousTripStateKey)
	if previous == nil {
		previous = data.NewMap()
	}
	response.Code = HTTPResponse.StatusCode
	response.TimeTakenMs = int(startEvent.Timestamp().Sub(endEvent.Timestamp()) / time.Millisecond)

	if toolbox.IsCompleteJSON(responseBody) {
		response.JSONBody, err = toolbox.JSONToMap(responseBody)
		if err == nil && HTTPRequest.Repeater != nil {
			_ = HTTPRequest.Variables.Apply(data.Map(response.JSONBody), previous)
		}
	}

	for k, v := range sendGroupResponse.Data {
		var expanded = previous.Expand(v)
		previous[k] = state.Expand(expanded)
	}

	err = repeater.Variables.Apply(previous, previous)
	if err != nil {
		return err
	}
	if len(previous) > 0 {
		state.Put(PreviousTripStateKey, previous)
	}
	if HTTPRequest.When != "" {
		return nil
	}

	for _, candidate := range sendGroupRequest.Requests {
		if candidate.When != "" && strings.Contains(response.Body, candidate.When) {
			err = s.sendRequest(context, client, candidate, sessionCookies, sendGroupRequest, sendGroupResponse)
			if err != nil {
				return err
			}
		}
	}
	return nil
}

func replaceResponseBodyIfNeeded(sendHTTPRequest *Request, responseBody string) string {
	if len(sendHTTPRequest.Replace) > 0 {
		for k, v := range sendHTTPRequest.Replace {
			responseBody = strings.Replace(responseBody, k, v, len(responseBody))
		}
	}
	return responseBody
}

func (s *service) applyDefaultTimeoutIfNeeded(options []*toolbox.HttpOptions) []*toolbox.HttpOptions {
	if len(options) > 0 {
		return options
	}
	return []*toolbox.HttpOptions{
		{
			Key:   "RequestTimeoutMs",
			Value: 120000,
		},
		{
			Key:   "TimeoutMs",
			Value: 120000,
		},
	}
}

func (s *service) send(context *endly.Context, request *SendRequest) (*SendResponse, error) {
	client, err := toolbox.NewHttpClient(s.applyDefaultTimeoutIfNeeded(request.Options)...)
	if err != nil {
		return nil, fmt.Errorf("failed to send req: %v", err)
	}
	defer s.resetContext(context, request)
	var result = &SendResponse{
		Responses: make([]*Response, 0),
		Data:      make(map[string]interface{}),
	}
	var sessionCookies Cookies = make([]*http.Cookie, 0)
	var state = context.State()
	if !state.Has("cookies") {
		state.Put("cookies", data.NewMap())
	}
	for _, req := range request.Requests {
		if req.When != "" {
			continue
		}
		err = s.sendRequest(context, client, req, &sessionCookies, request, result)
		if err != nil {
			return nil, err
		}
	}

	if request.Expect != nil {
		var actual = map[string]interface{}{
			"Responses": result.Responses,
			"Data":      result.Data,
		}
		result.Assert, err = validator.Assert(context, request, request.Expect, actual, "HTTP.responses", "assert http responses")
	}

	return result, nil

}
func readBody(httpResponse *http.Response, response *Response, expectBased64Encoded bool) {

	body, err := ioutil.ReadAll(httpResponse.Body)
	if err != nil {
		response.Error = fmt.Sprintf("%v", err)
		return
	}
	httpResponse.Body.Close()
	if expectBased64Encoded {
		buf := new(bytes.Buffer)
		encoder := base64.NewEncoder(base64.StdEncoding, buf)
		_, _ = encoder.Write(body)
		_ = encoder.Close()
		response.Body = "base64:" + string(buf.Bytes())

	} else {
		response.Body = string(body)
	}
}

func copyHeaders(source http.Header, target http.Header) {
	for key, values := range source {
		if _, has := target[key]; !has {
			target[key] = make([]string, 0)
		}
		if len(values) == 1 {
			target.Set(key, values[0])
		} else {

			for _, value := range values {
				target.Add(key, value)
			}
		}
	}
}

func copyExpandedHeaders(source http.Header, target http.Header, context *endly.Context) {
	for key, values := range source {
		if _, has := target[key]; !has {
			target[key] = make([]string, 0)
		}
		if len(values) == 1 {
			target.Set(key, context.Expand(values[0]))
		} else {
			for _, value := range values {
				target.Add(key, context.Expand(value))
			}
		}
	}
}

//resetContext resets context for variables with Reset flag set, and removes PreviousTripStateKey
func (s *service) resetContext(context *endly.Context, request *SendRequest) {
	state := context.State()
	state.Delete(PreviousTripStateKey)
	for _, request := range request.Requests {
		if request.Repeater != nil && len(request.Extraction) > 0 {
			request.Extraction.Reset(state)
		}
	}
}

const httpRunnerSendRequestExample = `{
  "Requests": [
    {
      "Method": "GET",
      "URL": "http://127.0.0.1:8777/event1/?k1=v1\u0026k2=v2"
    },
    {
      "Method": "GET",
      "URL": "http://127.0.0.1:8777/event1/?k10=v1\u0026k2=v2"
    },
    {
      "When": "",
      "Method": "POST",
      "URL": "http://127.0.0.1:8777/event4/",
      "Body": "Lorem Ipsum is simply dummy text of the printing and typesetting industry."
    }
  ]
}`

func (s *service) registerRoutes() {
	s.Register(&endly.Route{
		Action: "send",
		RequestInfo: &endly.ActionInfo{
			Description: "send http request(s)",
			Examples: []*endly.UseCase{
				{
					Description: "send",
					Data:        httpRunnerSendRequestExample,
				},
			},
		},
		RequestProvider: func() interface{} {
			return &SendRequest{}
		},
		ResponseProvider: func() interface{} {
			return &SendResponse{}
		},
		Handler: func(context *endly.Context, request interface{}) (interface{}, error) {
			if req, ok := request.(*SendRequest); ok {

				return s.send(context, req)
			}
			return nil, fmt.Errorf("unsupported request type: %T", request)
		},
	})
}

//New creates a new http runner service
func New() endly.Service {
	var result = &service{
		AbstractService: endly.NewAbstractService(ServiceID),
	}
	result.AbstractService.Service = result
	result.registerRoutes()
	return result
}
