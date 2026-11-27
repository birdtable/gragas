// A part's stylesheet is imported for its effect: the bundler (the browser tests,
// the catalogue, the host's build) reads the file, and the type check only needs
// to know the import resolves.
declare module "*.css";
