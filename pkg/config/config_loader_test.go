package config

// // Env represents an interface for fetching environment variables.
// //
// //go:generate mockgen -source=$GOFILE -destination=mock_$GOFILE -package=$GOPACKAGE
// type Env interface {
// 	Get(key string) string
// }

// // Wd represents an interface for fetching the working directory.
// //
// //go:generate mockgen -source=$GOFILE -destination=mock_$GOFILE -package=$GOPACKAGE
// type Wd interface {
// 	Get() (string, error)
// }

// // Home represents an interface for fetching the home directory.
// //
// //go:generate mockgen -source=$GOFILE -destination=mock_$GOFILE -package=$GOPACKAGE
// type Home interface {
// 	Get() (string, error)
// }

// func TestConfigLoader_ReadConfig(t *testing.T) {
// 	ctrl := gomock.NewController(t)
// 	defer ctrl.Finish()

// 	mockViper := viper.New()
// 	mockEnv := NewMockEnv(ctrl)
// 	mockWd := NewMockWd(ctrl)
// 	mockHome := NewMockHome(ctrl)

// 	configLoader := NewConfigLoader(mockViper)
// 	configLoader.getEnv = mockEnv.Get
// 	configLoader.getWd = mockWd.Get
// 	configLoader.homeDir = mockHome.Get

// 	t.Run("load from environment variable", func(t *testing.T) {
// 		mockEnv.EXPECT().Get("ATMOS_CLI_CONFIG_PATH").Return("/env/path").Times(1)
// 		configFile, err := configLoader.ReadConfig("")
// 		assert.NoError(t, err)
// 		assert.Equal(t, filepath.Join("/env/path", "atmos.yaml"), configFile)
// 	})

// 	t.Run("load from home directory", func(t *testing.T) {
// 		mockHome.EXPECT().Get().Return("/home/user", nil).Times(1)
// 		configFile, err := configLoader.ReadConfig("")
// 		assert.NoError(t, err)
// 		assert.Equal(t, filepath.Join("/home/user", ".atmos", "atmos.yaml"), configFile)
// 	})

// 	t.Run("load from working directory", func(t *testing.T) {
// 		mockWd.EXPECT().Get().Return("/current/dir", nil).Times(1)
// 		configFile, err := configLoader.ReadConfig("")
// 		assert.NoError(t, err)
// 		assert.Equal(t, filepath.Join("/current/dir", "atmos.yaml"), configFile)
// 	})

// 	t.Run("fail to load from working directory", func(t *testing.T) {
// 		mockWd.EXPECT().Get().Return("", errors.New("failed to get working dir")).Times(1)
// 		configFile, err := configLoader.ReadConfig("")
// 		assert.Error(t, err)
// 		assert.Empty(t, configFile)
// 	})
// }
