package datafetcher

import "testing"

func TestInlineFetcher(t *testing.T) {
	t.Run("FetchData", func(t *testing.T) {
		t.Run("should return data when valid inline data is provided", func(t *testing.T) {
			fetcher := inlineJsonFetcher{}
			data, err := fetcher.FetchData(`{"key": "value"}`)
			if err != nil {
				t.Fatalf("Expected no error, got %v", err)
			}
			if string(data) != `{"key": "value"}` {
				t.Fatalf("Expected 'inline data', got %s", string(data))
			}
		})
		t.Run("should return error when invalid inline data is provided", func(t *testing.T) {
			fetcher := inlineJsonFetcher{}
			_, err := fetcher.FetchData("")
			if err == nil {
				t.Fatalf("Expected error, got none")
			}
		})
	})
}
