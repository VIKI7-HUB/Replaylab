import type { Config } from 'tailwindcss';

export default {
  content: ['./index.html', './src/**/*.{js,ts,jsx,tsx}'],
  darkMode: ['class', '[data-theme="dark"]'],
  theme: {
    colors: {
      transparent: 'transparent',
      current: 'currentColor',
      bg: 'var(--bg)',
      'bg-subtle': 'var(--bg-subtle)',
      'bg-muted': 'var(--bg-muted)',
      'bg-selected': 'var(--bg-selected)',
      border: 'var(--border)',
      'border-strong': 'var(--border-strong)',
      text: 'var(--text)',
      'text-2': 'var(--text-2)',
      'text-3': 'var(--text-3)',
      accent: 'var(--accent)',
      'accent-bg': 'var(--accent-bg)',
      success: 'var(--success)',
      'success-bg': 'var(--success-bg)',
      warning: 'var(--warning)',
      'warning-bg': 'var(--warning-bg)',
      danger: 'var(--danger)',
      'danger-bg': 'var(--danger-bg)',
    },
    borderRadius: {
      none: '0px',
      sm: '4px',
      DEFAULT: '6px',
      md: '8px',
      full: '9999px',
    },
    boxShadow: {
      none: 'none',
      float: 'var(--shadow-float)',
    },
    fontFamily: {
      sans: ['Geist Variable', 'sans-serif'],
      mono: ['Geist Mono Variable', 'monospace'],
    },
    // Motion per the design spec: opacity or a translate of 4px or less, 120-160ms.
    keyframes: {
      'fade-in': { from: { opacity: '0' }, to: { opacity: '1' } },
      'fade-out': { from: { opacity: '1' }, to: { opacity: '0' } },
      'rise-in': {
        from: { opacity: '0', transform: 'translateY(4px)' },
        to: { opacity: '1', transform: 'translateY(0)' },
      },
    },
    animation: {
      'fade-in': 'fade-in 140ms ease-out',
      'fade-out': 'fade-out 120ms ease-in forwards',
      'rise-in': 'rise-in 140ms ease-out',
    },
  },
  plugins: [],
} satisfies Config;

