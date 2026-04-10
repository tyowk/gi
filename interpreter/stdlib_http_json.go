package interpreter

import (
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

func runHTTP(req *http.Request, timeoutMS int64) (*Value, error) {
	client := &http.Client{Timeout: time.Duration(timeoutMS) * time.Millisecond}
	resp, err := client.Do(req)
	if err != nil {
		return nilVal(), err
	}
	defer resp.Body.Close()
	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return nilVal(), err
	}
	headers := map[string]*Value{}
	keys := make([]string, 0, len(resp.Header))
	for k, values := range resp.Header {
		keys = append(keys, k)
		headers[k] = strVal(strings.Join(values, ","))
	}
	return &Value{
		Kind: KindMap,
		MapVal: map[string]*Value{
			"status":     intVal(int64(resp.StatusCode)),
			"statusText": strVal(resp.Status),
			"body":       strVal(string(data)),
			"headers":    &Value{Kind: KindMap, MapVal: headers, MapKeys: keys},
		},
		MapKeys: []string{"status", "statusText", "body", "headers"},
	}, nil
}

func valueToAny(v *Value) interface{} {
	switch v.Kind {
	case KindNil:
		return nil
	case KindInt:
		return v.IntVal
	case KindFloat:
		return v.FltVal
	case KindString:
		return v.StrVal
	case KindBool:
		return v.BoolVal
	case KindArray:
		out := make([]interface{}, len(v.ArrVal))
		for i, item := range v.ArrVal {
			out[i] = valueToAny(item)
		}
		return out
	case KindMap, KindStruct:
		out := map[string]interface{}{}
		for _, k := range v.MapKeys {
			out[k] = valueToAny(v.MapVal[k])
		}
		return out
	}
	return v.String()
}

func anyToValue(v interface{}) *Value {
	switch t := v.(type) {
	case nil:
		return nilVal()
	case bool:
		return boolVal(t)
	case float64:
		if float64(int64(t)) == t {
			return intVal(int64(t))
		}
		return fltVal(t)
	case string:
		return strVal(t)
	case []interface{}:
		out := make([]*Value, len(t))
		for i, item := range t {
			out[i] = anyToValue(item)
		}
		return arrVal(out)
	case map[string]interface{}:
		mv := map[string]*Value{}
		keys := make([]string, 0, len(t))
		for k, item := range t {
			keys = append(keys, k)
			mv[k] = anyToValue(item)
		}
		return &Value{Kind: KindMap, MapVal: mv, MapKeys: keys}
	default:
		return strVal(fmt.Sprintf("%v", t))
	}
}
