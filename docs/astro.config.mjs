// @ts-check
import { defineConfig } from 'astro/config';
import starlight from '@astrojs/starlight';
import { rehypeHeadingIds } from '@astrojs/markdown-remark';
import rehypeAutolinkHeadings from 'rehype-autolink-headings';
import theme from 'toolbeam-docs-theme';

// https://astro.build/config
export default defineConfig({
	markdown: {
		rehypePlugins: [rehypeHeadingIds, [rehypeAutolinkHeadings, { behavior: 'wrap' }]],
	},
	integrations: [
		starlight({
			title: 'Wingman',
			favicon: '/WingmanBlue.png',
			expressiveCode: { themes: ['github-light', 'github-dark'] },
			customCss: [
				'@fontsource/roboto-mono/400.css',
				'@fontsource/roboto-mono/400-italic.css',
				'@fontsource/roboto-mono/500.css',
				'@fontsource/roboto-mono/600.css',
				'@fontsource/roboto-mono/700.css',
				'./src/styles/custom.css',
			],
			components: {
				SiteTitle: './src/components/SiteTitle.astro',
			},
			social: [
				{ icon: 'github', label: 'GitHub', href: 'https://github.com/chaserensberger/wingman' },
				{ icon: 'discord', label: 'Discord', href: 'https://discord.gg/Sxt68YGuZu' },
			],
			sidebar: [
				{ label: 'Introduction', slug: '' },
				{ label: 'Philosophy', slug: 'philosophy' },
				{ label: 'Architecture', slug: 'architecture' },
				{ label: 'Quickstart', slug: 'getting-started' },
				{ label: 'Demos', slug: 'demos' },
				{ label: 'SDK', slug: 'sdk' },
				{ label: 'Server', slug: 'server' },
				{ label: 'API', slug: 'api' },
				{
					label: 'WingModels',
					items: [
						{ label: 'Providers', slug: 'wingmodels/providers' },
						{ label: 'Parts', slug: 'wingmodels/parts' },
						{ label: 'Streaming', slug: 'wingmodels/streaming' },
					],
				},
				{
					label: 'WingHarness',
					items: [
						{ label: 'Agents', slug: 'wingharness/agents' },
						{ label: 'Sessions', slug: 'wingharness/sessions' },
						{ label: 'Tools', slug: 'wingharness/tools' },
						{ label: 'Lifecycle hooks', slug: 'wingharness/lifecycle' },
						{ label: 'Plugins', slug: 'wingharness/plugins' },
						{ label: 'Storage', slug: 'wingharness/storage' },
						{ label: 'Streaming', slug: 'wingharness/streaming' },
					],
				},
			],
			plugins: [
				theme({
					headerLinks: [],
				}),
			],
		}),
	],
});
