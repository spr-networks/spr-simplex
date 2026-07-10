import React from 'react'
import ReactDOM from 'react-dom/client'
import { PluginApp } from '@spr-networks/plugin-ui'
import Plugin from './Plugin'

// NOTE no StrictMode: gluestack-style's StyledProvider identifies the root
// provider by comparing a module-level id against useId(), and StrictMode's
// double render breaks the match — colorMode (dark mode) is never applied.
ReactDOM.createRoot(document.getElementById('root')).render(
  <PluginApp>
    <Plugin />
  </PluginApp>
)
