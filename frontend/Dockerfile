FROM node:20-alpine AS base
WORKDIR /app
COPY frontend/package.json ./
RUN npm install
COPY frontend ./

FROM base AS dev
CMD ["npm", "run", "dev", "--", "--host", "0.0.0.0", "--port", "3000"]

FROM base AS build
# Work around occasional broken npm ESM bin shim for vite inside Docker.
RUN node ./node_modules/vite/bin/vite.js build

FROM node:20-alpine AS prod
WORKDIR /app
COPY --from=build /app/build ./build
COPY frontend/package.json ./
RUN npm install --omit=dev
EXPOSE 3000
CMD ["node", "build"]
