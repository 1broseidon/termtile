// @ts-ignore — VitePress supports async config at runtime

async function getLatestVersion(repo: string): Promise<string | null> {
  try {
    const res = await fetch(`https://api.github.com/repos/1broseidon/${repo}/releases/latest`)
    if (!res.ok) return null
    const data = await res.json() as { tag_name: string }
    return data.tag_name ?? null
  } catch {
    return null
  }
}

export default (async () => {
  const version = await getLatestVersion('termtile')

  return {
    title: 'termtile',
    description: 'Tiling window manager with AI agent orchestration',
    base: '/termtile/',
    appearance: false,
    cleanUrls: true,

    head: [
      ['link', { rel: 'preconnect', href: 'https://fonts.googleapis.com' }],
      ['link', { rel: 'preconnect', href: 'https://fonts.gstatic.com', crossorigin: '' }],
      ['link', { href: 'https://fonts.googleapis.com/css2?family=Work+Sans:wght@300;400;700&family=JetBrains+Mono:wght@400;500&display=swap', rel: 'stylesheet' }],
      ['link', { rel: 'icon', type: 'image/x-icon', href: '/favicon.ico' }],
      ['link', { rel: 'icon', type: 'image/png', sizes: '32x32', href: '/favicon-32.png' }],
      ['link', { rel: 'icon', type: 'image/png', sizes: '16x16', href: '/favicon-16.png' }],
      ['link', { rel: 'apple-touch-icon', sizes: '192x192', href: '/termtile-192.png' }],
    ],

    themeConfig: {
      version,
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
  }
})
