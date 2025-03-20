package datafetcher

type inlineFetcher struct {
}

func (f inlineFetcher) FetchData(source string) ([]byte, error) {
	return []byte(source), nil
}
