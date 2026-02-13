import { defineConfig } from 'vitepress'

export default defineConfig({
  title: 'termtile',
  description: 'Tiling window manager with AI agent orchestration',

  head: [
    ['link', { rel: 'icon', type: 'image/x-icon', href: '/favicon.ico' }],
    ['link', { rel: 'icon', type: 'image/png', sizes: '32x32', href: '/favicon-32.png' }],
    ['link', { rel: 'icon', type: 'image/png', sizes: '16x16', href: '/favicon-16.png' }],
    ['link', { rel: 'apple-touch-icon', sizes: '192x192', href: '/termtile-192.png' }],
  ],

  themeConfig: {
    logo: '/termtile-logo.png',

    nav: [
      { text: 'Guide', link: '/getting-started' },
      { text: 'Config', link: '/configuration' },
      { text: 'Agents', link: '/agent-orchestration' },
      { text: 'GitHub', link: 'https://github.com/1broseidon/termtile' },
    ],

    sidebar: [
      {
        text: 'Guide',
        items: [
          { text: 'Getting Started', link: '/getting-started' },
          { text: 'Configuration', link: '/configuration' },
          { text: 'Layouts', link: '/layouts' },
          { text: 'Workspaces', link: '/workspaces' },
        ],
      },
      {
        text: 'AI Agents',
        items: [
          { text: 'Agent Orchestration', link: '/agent-orchestration' },
        ],
      },
      {
        text: 'Reference',
        items: [
          { text: 'CLI Reference', link: '/cli' },
          { text: 'TUI', link: '/tui' },
          { text: 'Daemon', link: '/daemon' },
        ],
      },
    ],

    socialLinks: [
      { icon: 'github', link: 'https://github.com/1broseidon/termtile' },
    ],

    footer: {
      message: 'Tiling window manager with AI agent orchestration',
    },
  },
})
