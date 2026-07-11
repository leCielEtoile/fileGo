/** @type {import('tailwindcss').Config} */
module.exports = {
  darkMode: 'class',
  // app.js はクラス名を文字列として動的生成するため、JSも走査対象に含める。
  content: [
    './web/templates/**/*.html',
    './web/static/js/**/*.js',
  ],
  theme: {
    extend: {
      colors: {
        // Google Drive 風のブルーをプライマリに。旧 discord 紫を置き換える。
        primary: {
          50:  '#e8f0fe',
          100: '#d2e3fc',
          200: '#aecbfa',
          500: '#1a73e8',
          600: '#1765cc',
          700: '#185abc',
        },
      },
    },
  },
  plugins: [],
};
