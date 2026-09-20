import { vitePreprocess } from '@sveltejs/vite-plugin-svelte'

// vitePreprocess gives <script lang="ts"> and the CSS Vite already handles.
export default { preprocess: vitePreprocess() }
