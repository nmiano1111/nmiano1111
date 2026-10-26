/** @type {import('tailwindcss').Config} */
export default {
  darkMode: 'class',
  content: [
    './index.html',
    './src/**/*.{js,ts,jsx,tsx}',
  ],
  theme: {
    extend: {
      colors: {
        // Catppuccin Mocha-inspired palette (matches CLI theme)
        base: {
          DEFAULT: '#1e1e2e',
          100: '#181825',
          200: '#1e1e2e',
          300: '#313244',
          400: '#45475a',
          500: '#585b70',
        },
        surface: {
          DEFAULT: '#1e1e2e',
          0: '#181825',
          1: '#1e1e2e',
          2: '#313244',
        },
        text: {
          DEFAULT: '#cdd6f4',
          dim: '#6c7086',
          subtle: '#9399b2',
        },
        accent: '#89b4fa',
        green: '#a6e3a1',
        red: '#f38ba8',
        yellow: '#f9e2af',
        mauve: '#cba6f7',
        peach: '#fab387',
        sky: '#89dceb',
        teal: '#94e2d5',
      },
      fontFamily: {
        sans: ['Inter', 'system-ui', 'sans-serif'],
        mono: ['JetBrains Mono', 'Fira Code', 'monospace'],
      },
      borderRadius: {
        DEFAULT: '6px',
      },
    },
  },
  plugins: [],
}
