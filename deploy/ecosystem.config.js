module.exports = {
  apps: [
    {
      name: "argus-server",
      script: "/app/argus",
      env: {
        ARGUS_ADDR: process.env.ARGUS_ADDR || ":4000",
        ARGUS_SLACK_WEBHOOK: process.env.ARGUS_SLACK_WEBHOOK || "",
        POSTGRES_URL: process.env.POSTGRES_URL || "",
        JWT_SECRET: process.env.JWT_SECRET || "",
        ARGUS_BASE_URL: process.env.ARGUS_BASE_URL || "http://localhost:4000",
        ARGUS_UI_URL: process.env.ARGUS_UI_URL || "http://localhost:3000",
        GITHUB_CLIENT_ID: process.env.GITHUB_CLIENT_ID || "",
        GITHUB_CLIENT_SECRET: process.env.GITHUB_CLIENT_SECRET || "",
        GOOGLE_CLIENT_ID: process.env.GOOGLE_CLIENT_ID || "",
        GOOGLE_CLIENT_SECRET: process.env.GOOGLE_CLIENT_SECRET || "",
      },
    },
    {
      name: "argus-ui",
      script: "node",
      args: "server.js",
      cwd: "/app/ui",
      env: {
        PORT: process.env.PORT || "3000",
        HOSTNAME: "0.0.0.0",
      },
    },
  ],
};
