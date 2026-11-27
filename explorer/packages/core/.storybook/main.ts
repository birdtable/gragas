// The parts catalogue (docs/architecture.md 8.2): Storybook over the stories
// that sit in each part's folder. It is a development tool and ships nothing.
// Storybook's own types are not imported: they do not pass the compiler under
// the strictest flags (see ../parts/stories.ts).
const config = {
  stories: ["../parts/**/*.stories.tsx", "../views/**/*.stories.tsx"],
  framework: "@storybook/react-vite",
  addons: [],
};

export default config;
