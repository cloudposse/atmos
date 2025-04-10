package datafetcher

import "encoding/json"

type inlineJsonFetcher struct{}

func (f inlineJsonFetcher) FetchData(source string) ([]byte, error) {
	data := make(map[string]interface{})
	err := json.Unmarshal([]byte(source), &data)
	if err != nil {
		return nil, err
	}
	return []byte(source), nil
}
