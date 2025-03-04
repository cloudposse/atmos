package downloader

import (
	"errors"
	"testing"
	"time"

	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/assert"
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
	expectedErr := ErrInvalidGitHubURL
	mockFactory.EXPECT().NewClient(gomock.Any(), "src", "dest", ClientModeFile).Return(nil, ErrInvalidGitHubURL)

	fd := NewFileDownloader(mockFactory)
	err := fd.Fetch("src", "dest", ClientModeFile, 10*time.Second)
	assert.Error(t, err)
	assert.Equal(t, expectedErr, errors.Unwrap(err))
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
