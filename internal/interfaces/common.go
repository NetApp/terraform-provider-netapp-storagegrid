// Package interfaces has the calls to the StorageGRID APIs and the models of the data that they send and return.
package interfaces

import "github.com/mitchellh/mapstructure"

// NameDataModel is the standard name/id pair that required by most resources
type NameDataModel struct {
	Name string
	ID   string
}

// decode converts a decoded JSON object into a struct, or a struct into a map, using the mapstructure tags.
// Types are converted when it is safe: the REST client keeps numbers as json.Number, and StorageGRID is not always
// consistent between strings and numbers (eg account ids).
func decode(input interface{}, output interface{}) error {
	decoder, err := mapstructure.NewDecoder(&mapstructure.DecoderConfig{
		WeaklyTypedInput: true,
		Result:           output,
	})
	if err != nil {
		return err
	}
	return decoder.Decode(input)
}
