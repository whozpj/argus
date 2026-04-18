/** @type {import('next').NextConfig} */
const nextConfig = {
  output: "standalone",
  async rewrites() {
    return [
      {
        source: "/api/:path*",
        destination: "http://localhost:4000/api/:path*",
      },
      {
        source: "/auth/:path*",
        destination: "http://localhost:4000/auth/:path*",
      },
      {
        source: "/healthz",
        destination: "http://localhost:4000/healthz",
      },
    ];
  },
};

export default nextConfig;
