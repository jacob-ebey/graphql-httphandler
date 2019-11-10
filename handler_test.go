package httphandler_test

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/graphql-go/graphql"

	core "github.com/jacob-ebey/graphql-core"

	httphandler "github.com/jacob-ebey/graphql-httphandler"
	"github.com/jacob-ebey/graphql-httphandler/schemas"
)

func TestWrappedErrorDefaultMessage(t *testing.T) {
	handler := httphandler.GraphQLHttpHandler{
		Executor: core.GraphQLExecutor{
			Schema: schemas.PingPongSchema,
		},
	}

	query := "query Test { ping }"

	jsonBody, err := json.Marshal(core.GraphQLRequest{
		Query:         query,
		OperationName: "Test",
		Variables: map[string]interface{}{
			"echo": "test",
		},
	})
	if err != nil {
		t.Fatal(err)
	}

	jsonStringVariablesBody, err := json.Marshal(map[string]interface{}{
		"query":         query,
		"operationName": "Test",
		"variables":     `{ "echo": "test" }`,
	})
	if err != nil {
		t.Fatal(err)
	}

	gqlRequest, err := http.NewRequest("POST", "/", strings.NewReader("query { ping }"))
	if err != nil {
		t.Fatal(err)
	}
	gqlRequest.Header.Set("Content-Type", httphandler.ContentTypeGraphQL)

	urlRequest, err := http.NewRequest("GET", fmt.Sprintf("/graphql?%v", `query=query Test { ping }&operationName=Test&variables={"echo": "test"}`), nil)
	if err != nil {
		t.Fatal(err)
	}

	jsonRequest, err := http.NewRequest("POST", "/", strings.NewReader(string(jsonBody)))
	if err != nil {
		t.Fatal(err)
	}
	jsonRequest.Header.Set("Content-Type", httphandler.ContentTypeJSON)

	jsonRequestNoContentType, err := http.NewRequest("POST", "/", strings.NewReader(string(jsonBody)))
	if err != nil {
		t.Fatal(err)
	}

	jsonRequestStringVariables, err := http.NewRequest("POST", "/", strings.NewReader(string(jsonStringVariablesBody)))
	if err != nil {
		t.Fatal(err)
	}

	cases := []*http.Request{
		gqlRequest,
		urlRequest,
		jsonRequest,
		jsonRequestNoContentType,
		jsonRequestStringVariables,
	}

	for _, request := range cases {
		resp := httptest.NewRecorder()

		handler.ServeHTTP(resp, request)

		if err != nil {
			t.Fatal(err)
		}

		if resp.Code != http.StatusOK {
			t.Fatal("Status code was not 200.")
		}

		body, err := ioutil.ReadAll(resp.Body)
		if err != nil {
			t.Fatal(err)
		}

		result := graphql.Result{}
		if err := json.Unmarshal(body, &result); err != nil {
			t.Fatal(err)
		}

		if result.HasErrors() {
			t.Fatal(result.Errors)
		}
	}
}
