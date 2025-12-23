package downloader

import (
	"errors"
	"testing"
	"time"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/stretchr/testify/assert"
	"go.uber.org/mock/gomock"
)

func TestFileDownloader_Fetch_Success(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockClient := NewMockDownloadClient(ctrl)
	mockFactory := NewMockClientFactory(ctrl)

	mockFactory.EXPECT().NewClient(gomock.Any(), "src", "dest", ClientModeFile).Return(mockClient, nil)
	mockClient.EXPECT().Get().Return(nil)

	fd := NewFileDownloader(mockFactory)
	err := fd.Fetch("src", "dest", ClientModeFile, 10*time.Second)
	assert.NoError(t, err)
}

func TestFileDownloader_Fetch_Failure(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockFactory := NewMockClientFactory(ctrl)
	expectedErr := errors.New("invalid URL")
	mockFactory.EXPECT().NewClient(gomock.Any(), "src", "dest", ClientModeFile).Return(nil, expectedErr)

	fd := NewFileDownloader(mockFactory)
	err := fd.Fetch("src", "dest", ClientModeFile, 10*time.Second)
	assert.Error(t, err)
	assert.ErrorIs(t, err, errUtils.ErrCreateDownloadClient)
}

func TestFileDownloader_FetchData(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockClient := NewMockDownloadClient(ctrl)
	mockFactory := NewMockClientFactory(ctrl)

	mockFactory.EXPECT().NewClient(gomock.Any(), "src", gomock.Any(), ClientModeFile).Return(mockClient, nil)
	mockClient.EXPECT().Get().Return(nil)
	fakeData := []byte(`{"some":"json"}`)
	tempFile := "/tmp/testfile.json"
	fd := &fileDownloader{
		clientFactory: mockFactory,
		tempPathGenerator: func() string {
			return tempFile
		},
		fileReader: func(_ string) ([]byte, error) {
			return fakeData, nil
		},
	}
	data, err := fd.FetchData("src")
	assert.NoError(t, err)
	assert.Equal(t, fakeData, data)
}

func TestFileDownloader_FetchAndAutoParse(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	testCases := []struct {
		name     string
		fileData string
	}{
		{
			name:     "should be able to parse json file",
			fileData: `{"some":"json"}`,
		},
		{
			name: "should be able to parse yaml file",
			fileData: `
test:
  hello: world
`,
		},
		{
			name:     "should be able to parse hcl file",
			fileData: `a = 1`,
		},
		{
			name:     "should be able to return data as is in case of unknown format",
			fileData: `some random data`,
		},
	}
	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			mockClient := NewMockDownloadClient(ctrl)
			mockFactory := NewMockClientFactory(ctrl)
			tempFile := "/tmp/testfile.json"
			mockFactory.EXPECT().NewClient(gomock.Any(), "src", tempFile, ClientModeFile).Return(mockClient, nil)
			mockClient.EXPECT().Get().Return(nil)

			fd := &fileDownloader{
				clientFactory: mockFactory,
				tempPathGenerator: func() string {
					return tempFile
				},
				fileReader: func(_ string) ([]byte, error) {
					return []byte(testCase.fileData), nil
				},
			}

			result, err := fd.FetchAndAutoParse("src")
			assert.NoError(t, err)
			assert.NotNil(t, result) // This assumes that the file content is valid JSON
		})
	}
}

func TestFileDownloader_FetchAndAutoParse_DownloadError(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockFactory := NewMockClientFactory(ctrl)
	expectedErr := errors.New("invalid URL")
	mockFactory.EXPECT().NewClient(gomock.Any(), "src", gomock.Any(), ClientModeFile).Return(nil, expectedErr)

	fd := NewFileDownloader(mockFactory)
	result, err := fd.FetchAndAutoParse("src")

	assert.Error(t, err)
	assert.Nil(t, result)
	assert.ErrorIs(t, err, errUtils.ErrDownloadFile)
}

func TestFileDownloader_FetchAndParseByExtension(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	testCases := []struct {
		name     string
		srcURL   string
		fileData string
	}{
		{
			name:     "JSON file by extension",
			srcURL:   "https://example.com/data.json",
			fileData: `{"key":"value"}`,
		},
		{
			name:     "YAML file by extension",
			srcURL:   "https://example.com/config.yaml",
			fileData: "key: value\n",
		},
		{
			name:     "Text file by extension",
			srcURL:   "https://example.com/readme.txt",
			fileData: "plain text content",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			mockClient := NewMockDownloadClient(ctrl)
			mockFactory := NewMockClientFactory(ctrl)
			tempFile := "/tmp/testfile"

			mockFactory.EXPECT().NewClient(gomock.Any(), tc.srcURL, tempFile, ClientModeFile).Return(mockClient, nil)
			mockClient.EXPECT().Get().Return(nil)

			fd := &fileDownloader{
				clientFactory: mockFactory,
				tempPathGenerator: func() string {
					return tempFile
				},
				fileReader: func(_ string) ([]byte, error) {
					return []byte(tc.fileData), nil
				},
			}

			result, err := fd.FetchAndParseByExtension(tc.srcURL)
			assert.NoError(t, err)
			assert.NotNil(t, result)
		})
	}
}

func TestFileDownloader_FetchAndParseByExtension_DownloadError(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockFactory := NewMockClientFactory(ctrl)
	expectedErr := errors.New("invalid URL")
	mockFactory.EXPECT().NewClient(gomock.Any(), "https://example.com/data.json", gomock.Any(), ClientModeFile).Return(nil, expectedErr)

	fd := NewFileDownloader(mockFactory)
	result, err := fd.FetchAndParseByExtension("https://example.com/data.json")

	assert.Error(t, err)
	assert.Nil(t, result)
	assert.ErrorIs(t, err, errUtils.ErrDownloadFile)
}

func TestFileDownloader_FetchAndParseRaw(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockClient := NewMockDownloadClient(ctrl)
	mockFactory := NewMockClientFactory(ctrl)
	tempFile := "/tmp/testfile"
	fileContent := "raw text content"

	mockFactory.EXPECT().NewClient(gomock.Any(), "src", tempFile, ClientModeFile).Return(mockClient, nil)
	mockClient.EXPECT().Get().Return(nil)

	fd := &fileDownloader{
		clientFactory: mockFactory,
		tempPathGenerator: func() string {
			return tempFile
		},
		fileReader: func(_ string) ([]byte, error) {
			return []byte(fileContent), nil
		},
	}

	result, err := fd.FetchAndParseRaw("src")
	assert.NoError(t, err)
	assert.Equal(t, fileContent, result)
}

func TestFileDownloader_FetchAndParseRaw_DownloadError(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockFactory := NewMockClientFactory(ctrl)
	expectedErr := errors.New("invalid URL")
	mockFactory.EXPECT().NewClient(gomock.Any(), "src", gomock.Any(), ClientModeFile).Return(nil, expectedErr)

	fd := NewFileDownloader(mockFactory)
	result, err := fd.FetchAndParseRaw("src")

	assert.Error(t, err)
	assert.Nil(t, result)
	assert.ErrorIs(t, err, errUtils.ErrDownloadFile)
}

func TestFileDownloader_FetchData_DownloadError(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockFactory := NewMockClientFactory(ctrl)
	expectedErr := errors.New("invalid URL")
	mockFactory.EXPECT().NewClient(gomock.Any(), "src", gomock.Any(), ClientModeFile).Return(nil, expectedErr)

	fd := NewFileDownloader(mockFactory)
	data, err := fd.FetchData("src")

	assert.Error(t, err)
	assert.Nil(t, data)
	assert.ErrorIs(t, err, errUtils.ErrDownloadFile)
}

func TestFileDownloader_FetchData_ReadError(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockClient := NewMockDownloadClient(ctrl)
	mockFactory := NewMockClientFactory(ctrl)
	tempFile := "/tmp/testfile"
	readErr := errors.New("file read error")

	mockFactory.EXPECT().NewClient(gomock.Any(), "src", tempFile, ClientModeFile).Return(mockClient, nil)
	mockClient.EXPECT().Get().Return(nil)

	fd := &fileDownloader{
		clientFactory: mockFactory,
		tempPathGenerator: func() string {
			return tempFile
		},
		fileReader: func(_ string) ([]byte, error) {
			return nil, readErr
		},
	}

	data, err := fd.FetchData("src")
	assert.Error(t, err)
	assert.Nil(t, data)
	assert.Equal(t, readErr, err)
}
