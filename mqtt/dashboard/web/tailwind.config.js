// Tailwind v3 config - Vercel-themed (tweakcn) tokens via CSS variables.
module.exports = {
  darkMode: 'class',
  content: ['./mqtt/dashboard/templates/**/*.html'],
  // .dark must be safelisted - shadcn-style theming defines it in input.css
  // (the rule that flips the CSS variables) but Tailwind would otherwise
  // purge it as "not used by any utility class".
  safelist: ['sparkline', 'dark'],
  theme: {
    extend: {
      colors: {
        background:               'hsl(var(--background))',
        foreground:               'hsl(var(--foreground))',
        card:                     'hsl(var(--card))',
        'card-foreground':        'hsl(var(--card-foreground))',
        popover:                  'hsl(var(--popover))',
        'popover-foreground':     'hsl(var(--popover-foreground))',
        primary:                  'hsl(var(--primary))',
        'primary-foreground':     'hsl(var(--primary-foreground))',
        secondary:                'hsl(var(--secondary))',
        'secondary-foreground':   'hsl(var(--secondary-foreground))',
        muted:                    'hsl(var(--muted))',
        'muted-foreground':       'hsl(var(--muted-foreground))',
        accent:                   'hsl(var(--accent))',
        'accent-foreground':      'hsl(var(--accent-foreground))',
        destructive:              'hsl(var(--destructive))',
        'destructive-foreground': 'hsl(var(--destructive-foreground))',
        success:                  'hsl(var(--success))',
        'success-foreground':     'hsl(var(--success-foreground))',
        border:                   'hsl(var(--border))',
        input:                    'hsl(var(--input))',
        ring:                     'hsl(var(--ring))',
      },
      borderRadius: {
        lg: 'var(--radius)',
        md: 'calc(var(--radius) - 2px)',
        sm: 'calc(var(--radius) - 4px)',
      },
      fontFamily: {
        sans: ['ui-sans-serif', 'system-ui', '-apple-system', 'BlinkMacSystemFont', '"Segoe UI"', 'Roboto', '"Helvetica Neue"', 'Arial', 'sans-serif'],
        mono: ['ui-monospace', 'SFMono-Regular', 'Menlo', 'Monaco', 'Consolas', '"Liberation Mono"', '"Courier New"', 'monospace'],
      },
    },
  },
  plugins: [],
};
