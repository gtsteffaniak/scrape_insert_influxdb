package query

import (
	"strconv"

	"github.com/PaesslerAG/jsonpath"
)

func ExtractValueUsingJSONQuery(data interface{}, query string) string {
	value, err := jsonpath.Get(query, data)
	if err != nil {
		return ""
	}
	switch v := value.(type) {
	case string:
		return v
	case int:
		return strconv.Itoa(v)
	case float64:
		return strconv.FormatFloat(v, 'f', -1, 64)
	case []interface{}:
		if len(v) > 0 {
			return ExtractValueUsingJSONQuery(v[0], "$")
		}
		return ""
	default:
		return ""
	}
}
