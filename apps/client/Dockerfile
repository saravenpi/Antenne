# --- build ---
FROM node:22-bookworm-slim AS build
WORKDIR /app
COPY apps/client/package.json apps/client/package-lock.json* ./
RUN npm install
COPY apps/client/ ./
ARG PUBLIC_API_URL=/api
ENV PUBLIC_API_URL=$PUBLIC_API_URL
RUN npm run build

# --- runtime (adapter-node) ---
FROM node:22-bookworm-slim
WORKDIR /app
ENV NODE_ENV=production
COPY --from=build /app/build ./build
COPY --from=build /app/package.json ./
COPY --from=build /app/node_modules ./node_modules
EXPOSE 3000
ENV PORT=3000
CMD ["node", "build"]
