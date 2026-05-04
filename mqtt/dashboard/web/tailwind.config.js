// mqtt/dashboard/web/tailwind.config.js
module.exports = {
  darkMode: 'class',
  content: ['./mqtt/dashboard/templates/**/*.html'],
  theme: {
    extend: {
      colors: {
        // EMQX-ish accent
        brand: {
          50:  '#eff6ff',
          100: '#dbeafe',
          500: '#3b82f6',
          600: '#2563eb',
          700: '#1d4ed8',
        },
      },
    },
  },
  plugins: [],
};
