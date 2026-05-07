import { defineConfig } from 'vitepress'

export default defineConfig({
  title: 'GHA Cache Server',
  description: 'Self-hosted GitHub Actions cache server. Drop-in compatible with actions/cache@v4 and @v5.',
  cleanUrls: true,
  themeConfig: {
    nav: [
      { text: 'Guide', link: '/getting-started' },
    ],
    sidebar: [
      {
        text: 'Guide',
        items: [
          { text: 'Getting Started', link: '/getting-started' },
          { text: 'How it works', link: '/how-it-works' },
          { text: 'Runner Setup', link: '/runner-setup' },
          { text: 'Helm Chart', link: '/helm' },
          { text: 'Management API', link: '/management-api' },
        ],
      },
      {
        text: 'Storage Drivers',
        items: [
          { text: 'File System', link: '/storage-drivers/file-system' },
          { text: 'S3 / MinIO', link: '/storage-drivers/s3' },
          { text: 'Google Cloud Storage', link: '/storage-drivers/google-cloud-storage' },
        ],
      },
      {
        text: 'Database Drivers',
        items: [
          { text: 'SQLite', link: '/database-drivers/sqlite' },
          { text: 'PostgreSQL', link: '/database-drivers/postgres' },
          { text: 'MySQL', link: '/database-drivers/mysql' },
        ],
      },
    ],
  },
})
