// A fixture imported as text (`?raw`): the bundler reads the file, and the type
// check only needs to know the import resolves to its text.
declare module "*?raw" {
  const text: string;
  // eslint-disable-next-line no-restricted-syntax -- the bundler's `?raw` import is a default export by its own contract
  export default text;
}
