module.exports = {
  apps: [
    {
      name: "argus-server",
      script: "/app/argus",
      env: {
        ARGUS_ADDR: process.env.ARGUS_ADDR || ":4000",
        ARGUS_DB_PATH: process.env.ARGUS_DB_PATH || "/data/argus.db",
        ARGUS_SLACK_WEBHOOK: process.env.ARGUS_SLACK_WEBHOOK || "",
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
        NEXT_PUBLIC_ARGUS_SERVER: "http://localhost:4000",
      },
    },
  ],
};
