const { skeleton } = require('@skeletonlabs/tw-plugin');

module.exports = {
  content: ['./src/**/*.{html,js,svelte,ts}'],
  theme: {
    extend: {}
  },
  plugins: [skeleton({ themes: { preset: ['skeleton'] } })]
};
