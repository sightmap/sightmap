/** @type {import('next').NextConfig} */
const nextConfig = {
  // Nothing Sightmap-specific is needed here. The corpus and the compiled tool
  // layer are plain static files under public/.well-known/, written by
  // `sightmap-next build` (wired as the `prebuild` script).
};
export default nextConfig;
