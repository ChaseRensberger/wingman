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
			],
			plugins: [
				theme({
					headerLinks: [],
				}),
			],
		}),
	],
});
