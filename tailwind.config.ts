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
  },
  plugins: [],
} satisfies Config;

