import { defineConfig } from 'astro/config';
import starlight from '@astrojs/starlight';

export default defineConfig({
  site: 'https://oguzhankrcb.github.io/HostShift',
  integrations: [
    starlight({
      title: 'HostShift',
      description: 'Read-only-source Ubuntu and Debian server migration documentation.',
      disable404Route: true,
      editLink: {
        baseUrl: 'https://github.com/oguzhankrcb/HostShift/edit/main/docs-site/'
      },
      social: [
        {
          icon: 'github',
          label: 'GitHub',
          href: 'https://github.com/oguzhankrcb/HostShift'
        }
      ],
      sidebar: [
        {
          label: 'Start Here',
          items: [
            { slug: 'overview' },
            { slug: 'getting-started/install' },
            { slug: 'getting-started/quick-start' },
            { slug: 'concepts/source-safety' }
          ]
        },
        {
          label: 'Migration Guides',
          items: [
            { slug: 'guides/profiles' },
            { slug: 'guides/workloads' },
            { slug: 'guides/validation' },
            { slug: 'examples/web-stack' }
          ]
        },
        {
          label: 'Operations',
          items: [
            { slug: 'operations/docker-compose' },
            { slug: 'operations/release' },
            { slug: 'operations/self-hosted-runner' }
          ]
        },
        {
          label: 'Reference',
          items: [
            { slug: 'reference/architecture' },
            { slug: 'reference/cli' },
            { slug: 'reference/profile-v2' },
            { slug: 'reference/source-discovery' },
            { slug: 'reference/workloads' },
            { slug: 'reference/checks' },
            { slug: 'reference/platforms' },
            { slug: 'reference/plans-state' },
            { slug: 'reference/test-matrix' },
            { slug: 'reference/threat-model' },
            { slug: 'reference/security' },
            { slug: 'reference/contributing' }
          ]
        }
      ]
    })
  ]
});
