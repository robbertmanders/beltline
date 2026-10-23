// Package jsonx holds the JSON conventions shared by the wire and the data
// folder: dates as ISO 8601 in UTC, nil slices and maps written as [] and {},
// and "<", ">" and "&" left unescaped. Otherwise it writes what encoding/json
// writes: struct fields in declaration order, map keys sorted. The encoder
// builds one plan per type from its struct tags and reuses it.
package jsonx
