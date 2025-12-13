package downloader

import (
	"errors"
	"os"
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
	assert.Contains(t, err.Error(), "failed to download file")
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
	assert.Contains(t, err.Error(), "failed to download file")
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
	assert.Contains(t, err.Error(), "failed to download file")
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
	assert.Contains(t, err.Error(), "failed to download file")
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

func TestFileDownloader_Fetch_GetError(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockClient := NewMockDownloadClient(ctrl)
	mockFactory := NewMockClientFactory(ctrl)
	expectedErr := errors.New("network error")

	mockFactory.EXPECT().NewClient(gomock.Any(), "src", "dest", ClientModeFile).Return(mockClient, nil)
	mockClient.EXPECT().Get().Return(expectedErr)

	fd := NewFileDownloader(mockFactory)
	err := fd.Fetch("src", "dest", ClientModeFile, 10*time.Second)
	assert.Error(t, err)
	assert.ErrorIs(t, err, errUtils.ErrDownloadFile)
}

func TestFileDownloader_FetchAtomic_Success(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockClient := NewMockDownloadClient(ctrl)
	mockFactory := NewMockClientFactory(ctrl)
	tempFile := "/tmp/testfile"
	fileContent := []byte("downloaded content")

	mockFactory.EXPECT().NewClient(gomock.Any(), "src", tempFile, ClientModeFile).Return(mockClient, nil)
	mockClient.EXPECT().Get().Return(nil)

	var writtenPath string
	var writtenData []byte

	fd := &fileDownloader{
		clientFactory: mockFactory,
		tempPathGenerator: func() string {
			return tempFile
		},
		fileReader: func(_ string) ([]byte, error) {
			return fileContent, nil
		},
		atomicWriter: func(path string, data []byte, _ os.FileMode) error {
			writtenPath = path
			writtenData = data
			return nil
		},
	}

	err := fd.FetchAtomic("src", "dest", ClientModeFile, 10*time.Second)
	assert.NoError(t, err)
	assert.Equal(t, "dest", writtenPath)
	assert.Equal(t, fileContent, writtenData)
}

func TestFileDownloader_FetchAtomic_CreateClientError(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockFactory := NewMockClientFactory(ctrl)
	expectedErr := errors.New("invalid URL")
	mockFactory.EXPECT().NewClient(gomock.Any(), "src", gomock.Any(), ClientModeFile).Return(nil, expectedErr)

	fd := NewFileDownloader(mockFactory)
	err := fd.FetchAtomic("src", "dest", ClientModeFile, 10*time.Second)
	assert.Error(t, err)
	assert.ErrorIs(t, err, errUtils.ErrCreateDownloadClient)
}

func TestFileDownloader_FetchAtomic_GetError(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockClient := NewMockDownloadClient(ctrl)
	mockFactory := NewMockClientFactory(ctrl)
	tempFile := "/tmp/testfile"
	expectedErr := errors.New("network error")

	mockFactory.EXPECT().NewClient(gomock.Any(), "src", tempFile, ClientModeFile).Return(mockClient, nil)
	mockClient.EXPECT().Get().Return(expectedErr)

	fd := &fileDownloader{
		clientFactory: mockFactory,
		tempPathGenerator: func() string {
			return tempFile
		},
	}

	err := fd.FetchAtomic("src", "dest", ClientModeFile, 10*time.Second)
	assert.Error(t, err)
	assert.ErrorIs(t, err, errUtils.ErrDownloadFile)
}

func TestFileDownloader_FetchAtomic_ReadError(t *testing.T) {
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

	err := fd.FetchAtomic("src", "dest", ClientModeFile, 10*time.Second)
	assert.Error(t, err)
	assert.ErrorIs(t, err, errUtils.ErrDownloadFile)
	// Verify cause error is in the chain.
	assert.ErrorIs(t, err, readErr)
}

func TestFileDownloader_FetchAtomic_WriteError(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockClient := NewMockDownloadClient(ctrl)
	mockFactory := NewMockClientFactory(ctrl)
	tempFile := "/tmp/testfile"
	fileContent := []byte("downloaded content")
	writeErr := errors.New("disk full")

	mockFactory.EXPECT().NewClient(gomock.Any(), "src", tempFile, ClientModeFile).Return(mockClient, nil)
	mockClient.EXPECT().Get().Return(nil)

	fd := &fileDownloader{
		clientFactory: mockFactory,
		tempPathGenerator: func() string {
			return tempFile
		},
		fileReader: func(_ string) ([]byte, error) {
			return fileContent, nil
		},
		atomicWriter: func(_ string, _ []byte, _ os.FileMode) error {
			return writeErr
		},
	}

	err := fd.FetchAtomic("src", "dest", ClientModeFile, 10*time.Second)
	assert.Error(t, err)
	assert.ErrorIs(t, err, errUtils.ErrDownloadFile)
	// Verify cause error is in the chain.
	assert.ErrorIs(t, err, writeErr)
}

func TestFileDownloader_FetchAtomic_InvalidClientMode(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockFactory := NewMockClientFactory(ctrl)
	fd := &fileDownloader{
		clientFactory:     mockFactory,
		tempPathGenerator: func() string { return "/tmp/testfile" },
		fileReader:        os.ReadFile,
		atomicWriter:      writeFileAtomicDefault,
	}

	// FetchAtomic should reject non-file modes.
	err := fd.FetchAtomic("src", "dest", ClientModeDir, 10*time.Second)
	assert.Error(t, err)
	assert.ErrorIs(t, err, errUtils.ErrInvalidClientMode)

	err = fd.FetchAtomic("src", "dest", ClientModeAny, 10*time.Second)
	assert.Error(t, err)
	assert.ErrorIs(t, err, errUtils.ErrInvalidClientMode)
}

func TestNewFileDownloader(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockFactory := NewMockClientFactory(ctrl)
	fd := NewFileDownloader(mockFactory)

	assert.NotNil(t, fd)
	// Type assert to access internal fields (for testing only).
	internalFD, ok := fd.(*fileDownloader)
	assert.True(t, ok)
	assert.NotNil(t, internalFD.clientFactory)
	assert.NotNil(t, internalFD.tempPathGenerator)
	assert.NotNil(t, internalFD.fileReader)
	assert.NotNil(t, internalFD.atomicWriter)
}
