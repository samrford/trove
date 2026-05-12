// App version, read at build time from frontend/package.json. Vite bundles
// the JSON import so this becomes a constant in the output.
import pkg from '../../package.json';

export const APP_VERSION: string = pkg.version;
