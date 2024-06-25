// https://stackoverflow.com/questions/74876229/how-to-tweak-docusaurus-webpack-config-for-some-react-component
// https://webpack.js.org/loaders/html-loader/
// https://github.com/algolia/docsearch-website/blob/master/plugins/my-loaders/index.js
// https://github.com/facebook/docusaurus/issues/2097
// https://webpack.js.org/concepts/#loaders

const html = require('html-loader');
const path = require('path');

module.exports = function (context, options) {
    return {
      name: 'custom-loaders',
      configureWebpack(config, isServer) {
        return {
          /*output: {
            filename: 'custom-loaders-webpack.bundle.js',
          },*/
        
          module: {
            rules: [
              // { test: /\.txt$/, use: 'raw-loader' },
              // https://webpack.js.org/loaders/html-loader/
              {
                test: /\.(html|htm|txt)$/i,
                loader: "html-loader",
                options: {
                  minimize: {
                    removeComments: false,
                    collapseWhitespace: false,
                  },
                },
              },
              {
                test: /\.(yml|yaml|tf)$/,
                use: 'raw-loader'
              }
            ],
          },

          resolve: {
            alias: {
              '@examples': path.resolve(__dirname, 'examples'),
            }
          }
        };
      },
    };
  };
