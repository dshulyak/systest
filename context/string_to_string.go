package context

import (
	"bytes"
	"fmt"
	"strings"
)

// StringToStringValue is a flag type for string to uint64 values.
type StringToStringValue struct {
	value   map[string]string
	changed bool
}

// newStringToStringValue creates instance.
func newStringToStringValue(p map[string]string) *StringToStringValue {
	return &StringToStringValue{value: p}
}

// Set expects value as "smth=101,else=102".
func (s *StringToStringValue) Set(val string) error {
	ss := strings.Split(val, ",")
	for _, pair := range ss {
		parts := strings.SplitN(pair, "=", 2)
		if len(parts) != 2 {
			return fmt.Errorf("%s must be formatted as key=value", pair)
		}
		key := strings.TrimSpace(parts[0])
		out := strings.TrimSpace(parts[1])
		s.value[key] = out
	}
	return nil
}

// Type returns stringToUint64 type.
func (s *StringToStringValue) Type() string {
	return "string=string"
}

// String marshals value of the StringToUint64Value instance.
func (s *StringToStringValue) String() string {
	var buf bytes.Buffer
	for k, v := range s.value {
		buf.WriteString(k)
		buf.WriteRune('=')
		buf.WriteString(v)
		buf.WriteRune(',')
	}
	if buf.Len() == 0 {
		return ""
	}
	return buf.String()[:buf.Len()-1]
}
