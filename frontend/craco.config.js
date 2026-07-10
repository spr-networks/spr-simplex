const InlineChunkHtmlPlugin = require('react-dev-utils/InlineChunkHtmlPlugin')

module.exports = {
  webpack: {
    configure: (webpackConfig) => {
      webpackConfig.plugins.forEach((plugin) => {
        if (plugin instanceof InlineChunkHtmlPlugin) {
          plugin.tests = [/.+[.]js/]
          plugin.options = { inject: 'body' }
        }
      })

      const transpileModules = [
        /node_modules[\\/]@gluestack-ui[\\/]/,
        /node_modules[\\/]@gluestack-style[\\/]/,
        /node_modules[\\/]@legendapp[\\/]/,
        /node_modules[\\/]@spr-networks[\\/]plugin-ui[\\/]/
      ]
      const oneOf = webpackConfig.module.rules.find((rule) => rule.oneOf).oneOf
      const appBabel = oneOf.find(
        (loader) =>
          loader.loader && loader.loader.includes('babel-loader') && loader.include
      )
      oneOf.unshift({
        test: /\.(js|mjs|jsx)$/,
        include: transpileModules,
        loader: appBabel.loader,
        options: { ...appBabel.options, sourceType: 'unambiguous' }
      })

      const oneOfRuleIdx = webpackConfig.module.rules.findIndex((rule) => !!rule.oneOf)
      webpackConfig.module.rules[oneOfRuleIdx].oneOf.forEach((loader) => {
        if (
          loader.test &&
          loader.test.test &&
          (loader.test.test('test.module.css') || loader.test.test('test.module.scss'))
        ) {
          loader.use.forEach((use) => {
            if (use.loader && use.loader.includes('mini-css-extract-plugin')) {
              use.loader = require.resolve('style-loader')
            }
          })
        }
      })

      return webpackConfig
    }
  }
}
