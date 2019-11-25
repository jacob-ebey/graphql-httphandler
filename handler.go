package httphandler

import (
	"context"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"net/url"
	"strconv"
	"strings"

	core "github.com/jacob-ebey/graphql-core"
)

const (
	ContentTypeJSON              = "application/json"
	ContentTypeGraphQL           = "application/graphql"
	ContentTypeFormURLEncoded    = "application/x-www-form-urlencoded"
	ContentTypeMultipartFormData = "multipart/form-data"
)

// a workaround for getting`variables` as a JSON string
type graphQLRequestCompatibility struct {
	Query         string `json:"query" url:"query" schema:"query"`
	OperationName string `json:"operationName" url:"operationName" schema:"operationName"`
	Variables     string `json:"variables" url:"variables" schema:"variables"`
}

func getFromForm(values url.Values) *core.GraphQLRequest {
	query := values.Get("query")
	if query != "" {
		// get variables map
		variables := make(map[string]interface{}, len(values))
		variablesStr := values.Get("variables")
		json.Unmarshal([]byte(variablesStr), &variables)

		return &core.GraphQLRequest{
			Query:         query,
			Variables:     variables,
			OperationName: values.Get("operationName"),
		}
	}

	return nil
}

func newGraphQLRequest(r *http.Request) core.GraphQLRequest {
	if reqOpt := getFromForm(r.URL.Query()); reqOpt != nil {
		return *reqOpt
	}

	if r.Method != http.MethodPost {
		return core.GraphQLRequest{}
	}

	if r.Body == nil {
		return core.GraphQLRequest{}
	}

	contentTypeStr := r.Header.Get("Content-Type")
	contentTypeTokens := strings.Split(contentTypeStr, ";")
	contentType := contentTypeTokens[0]

	switch contentType {
	case ContentTypeGraphQL:
		body, err := ioutil.ReadAll(r.Body)
		if err != nil {
			return core.GraphQLRequest{}
		}
		return core.GraphQLRequest{
			Query: string(body),
		}
	case ContentTypeFormURLEncoded:
		if err := r.ParseForm(); err != nil {
			return core.GraphQLRequest{}
		}

		fmt.Println(r.PostForm)
		if reqOpt := getFromForm(r.PostForm); reqOpt != nil {
			return *reqOpt
		}

		return core.GraphQLRequest{}
	case ContentTypeMultipartFormData:
		if err := r.ParseMultipartForm(1024 * 1024 * 16); err != nil {
			// fmt.Printf("Parse Multipart Failed %v", err)
			return core.GraphQLRequest{}
		}

		// @TODO handle array case...

		operationsParam := r.FormValue("operations")
		opts := core.GraphQLRequest{}
		if err := json.Unmarshal([]byte(operationsParam), &opts); err != nil {
			// fmt.Printf("Parse Operations Failed %v", err)
			return core.GraphQLRequest{}
		}

		mapParam := r.FormValue("map")
		mapValues := make(map[string]([]string))
		if len(mapParam) != 0 {
			if err := json.Unmarshal([]byte(mapParam), &mapValues); err != nil {
				// fmt.Printf("Parse map Failed %v", err)
				return core.GraphQLRequest{}
			}
		}

		variables := opts

		for key, value := range mapValues {
			for _, v := range value {
				if file, header, err := r.FormFile(key); err == nil {

					// Now set the path in ther variables
					var node interface{} = variables

					parts := strings.Split(v, ".")
					last := parts[len(parts)-1]

					for _, vv := range parts[:len(parts)-1] {
						// fmt.Printf("Doing vv=%s type=%T parts=%v\n", vv, node, parts)
						switch node.(type) {
						case core.GraphQLRequest:
							if vv == "variables" {
								node = opts.Variables
							} else {
								// panic("Invalid top level tag")
								return core.GraphQLRequest{}
							}
						case map[string]interface{}:
							node = node.(map[string]interface{})[vv]
						case []interface{}:
							if idx, err := strconv.ParseInt(vv, 10, 64); err == nil {
								node = node.([]interface{})[idx]
							} else {
								// panic("Unable to lookup index")
								return core.GraphQLRequest{}
							}
						default:
							// panic(fmt.Errorf("Unknown type %T", node))
							return core.GraphQLRequest{}
						}
					}

					data := &core.MultipartFile{File: file, Header: header}

					switch node.(type) {
					case map[string]interface{}:
						node.(map[string]interface{})[last] = data
					case []interface{}:
						if idx, err := strconv.ParseInt(last, 10, 64); err == nil {
							node.([]interface{})[idx] = data
						} else {
							// panic("Unable to lookup index")
							return core.GraphQLRequest{}
						}
					default:
						// panic(fmt.Errorf("Unknown last type %T", node))
						return core.GraphQLRequest{}
					}
				}
			}
		}

		return opts

	case ContentTypeJSON:
		fallthrough
	default:
		opts := core.GraphQLRequest{}
		body, err := ioutil.ReadAll(r.Body)
		if err != nil {
			return opts
		}
		err = json.Unmarshal(body, &opts)
		if err != nil {
			// Probably `variables` was sent as a string instead of an object.
			// So, we try to be polite and try to parse that as a JSON string
			var optsCompatible graphQLRequestCompatibility
			json.Unmarshal(body, &optsCompatible)
			json.Unmarshal([]byte(optsCompatible.Variables), &opts.Variables)
		}
		return opts
	}
}

type GraphQLHttpHandler struct {
	Executor   core.GraphQLExecutor
	Playground bool
}

func (handler *GraphQLHttpHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	acceptHeader := r.Header.Get("Accept")
	if handler.Playground {
		_, raw := r.URL.Query()["raw"]
		if !raw && !strings.Contains(acceptHeader, "application/json") && strings.Contains(acceptHeader, "text/html") {
			renderPlayground(w, r)
			return
		}
	}

	ctx := context.WithValue(r.Context(), "request", r)

	request := newGraphQLRequest(r)
	result := handler.Executor.Execute(ctx, request)

	json, err := json.Marshal(result)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	w.Header().Add("Content-Type", "application/json; charset=utf-8")
	w.Write(json)
}
