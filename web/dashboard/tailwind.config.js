/** @type {import('tailwindcss').Config} */
export default {
  content: [
    "./index.html",
    "./src/**/*.{svelte,js,ts,jsx,tsx}",
  ],
  darkMode: 'class',
  theme: {
    extend: {
      colors: {
        // Night Owl Light Theme
        'light': {
          'bg': '#FBFBFB',
          'bg-secondary': '#F0F0F0',
          'bg-tertiary': '#E4E4E4',
          'text': '#403F53',
          'text-secondary': '#5C6773',
          'text-muted': '#989FB1',
          'border': '#E4E4E4',
        },
        // Night Owl Dark Theme
        'dark': {
          'bg': '#011627',
          'bg-secondary': '#0B253A',
          'bg-tertiary': '#122D42',
          'text': '#D6DEEB',
          'text-secondary': '#A9B2C3',
          'text-muted': '#637777',
          'border': '#122D42',
        },
        // Accent colors
        'accent': {
          'blue': '#0077AA',
          'green': '#08916A',
          'purple': '#994CC3',
          'orange': '#AA5D00',
          'red': '#E64100',
          'yellow': '#C96765',
          'cyan': '#0C969B',
        },
        // Dark accent colors
        'accent-dark': {
          'blue': '#82AAFF',
          'green': '#22DA6E',
          'purple': '#C792EA',
          'orange': '#F78C6C',
          'red': '#FF6363',
          'yellow': '#FFCB8B',
          'cyan': '#7FDBCA',
        }
      },
      fontFamily: {
        'sans': ['Inter', 'system-ui', '-apple-system', 'BlinkMacSystemFont', 'Segoe UI', 'Roboto', 'sans-serif'],
        'mono': ['JetBrains Mono', 'Consolas', 'Monaco', 'Courier New', 'monospace'],
      },
      animation: {
        'fade-in': 'fadeIn 0.5s ease-in-out',
        'slide-up': 'slideUp 0.3s ease-out',
        'slide-down': 'slideDown 0.3s ease-out',
        'scale-in': 'scaleIn 0.2s ease-out',
        'pulse-slow': 'pulse 3s cubic-bezier(0.4, 0, 0.6, 1) infinite',
      },
      keyframes: {
        fadeIn: {
          '0%': { opacity: '0' },
          '100%': { opacity: '1' },
        },
        slideUp: {
          '0%': { transform: 'translateY(10px)', opacity: '0' },
          '100%': { transform: 'translateY(0)', opacity: '1' },
        },
        slideDown: {
          '0%': { transform: 'translateY(-10px)', opacity: '0' },
          '100%': { transform: 'translateY(0)', opacity: '1' },
        },
        scaleIn: {
          '0%': { transform: 'scale(0.95)', opacity: '0' },
          '100%': { transform: 'scale(1)', opacity: '1' },
        },
      },
      backdropBlur: {
        xs: '2px',
      }
    },
  },
  plugins: [],
}