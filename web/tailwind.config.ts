import type { Config } from "tailwindcss";

export default {
	darkMode: ["class"],
	content: [
		"./src/**/*.{ts,tsx}",
	],
	prefix: "",
	theme: {
		// One family everywhere: IBM Plex Mono, bundled via @font-face in index.css.
		fontFamily: {
			// --font is set by a theme that brings its own type (Amber); others use Plex.
			sans: ['var(--font, "IBM Plex Mono")', 'ui-monospace', 'SF Mono', 'Menlo', 'monospace'],
			mono: ['var(--font, "IBM Plex Mono")', 'ui-monospace', 'SF Mono', 'Menlo', 'monospace'],
			wordmark: ['var(--font, "IBM Plex Mono")', 'ui-monospace', 'monospace'],
		},
		extend: {
			colors: {
				border: "hsl(var(--border) / <alpha-value>)",
				input: "hsl(var(--input) / <alpha-value>)",
				ring: "hsl(var(--ring) / <alpha-value>)",
				background: "hsl(var(--background) / <alpha-value>)",
				foreground: "hsl(var(--foreground) / <alpha-value>)",
				primary: {
					DEFAULT: "hsl(var(--primary) / <alpha-value>)",
					foreground: "hsl(var(--primary-foreground) / <alpha-value>)",
				},
				secondary: {
					DEFAULT: "hsl(var(--secondary) / <alpha-value>)",
					foreground: "hsl(var(--secondary-foreground) / <alpha-value>)",
				},
				destructive: {
					DEFAULT: "hsl(var(--destructive) / <alpha-value>)",
					foreground: "hsl(var(--destructive-foreground) / <alpha-value>)",
				},
				muted: {
					DEFAULT: "hsl(var(--muted) / <alpha-value>)",
					foreground: "hsl(var(--muted-foreground) / <alpha-value>)",
				},
				accent: {
					DEFAULT: "hsl(var(--accent) / <alpha-value>)",
					foreground: "hsl(var(--accent-foreground) / <alpha-value>)",
				},
				popover: {
					DEFAULT: "hsl(var(--popover) / <alpha-value>)",
					foreground: "hsl(var(--popover-foreground) / <alpha-value>)",
				},
				card: {
					DEFAULT: "hsl(var(--card) / <alpha-value>)",
					foreground: "hsl(var(--card-foreground) / <alpha-value>)",
				},
				text: "hsl(var(--text) / <alpha-value>)",
				subtle: "hsl(var(--subtle) / <alpha-value>)",
				dim: "hsl(var(--dim) / <alpha-value>)",
				line: "hsl(var(--line) / <alpha-value>)",
				raise: "hsl(var(--raise) / <alpha-value>)",
				error: "hsl(var(--error) / <alpha-value>)",
			},
			borderRadius: {
				lg: "var(--radius)",
				md: "calc(var(--radius) - 2px)",
				sm: "calc(var(--radius) - 4px)",
			},
		},
	},
	plugins: [],
} satisfies Config;
