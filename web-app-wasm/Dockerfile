FROM node:21-alpine AS deps
# Check https://github.com/nodejs/docker-node/tree/b4117f9333da4138b03a546ec926ef50a31506c3#nodealpine to understand why libc6-compat might be needed.
RUN apk add --no-cache libc6-compat
WORKDIR /app
COPY . .
RUN npm install
# TODO seankane: running the build:wasm step fails in a Dockerfile, so for now build everything locally and package into Dockerfile
# RUN npm run build:wasm
RUN npm run build

ENV NODE_ENV production

RUN addgroup -g 1001 -S nodejs
RUN adduser -S nextjs -u 1001

USER nextjs
EXPOSE 3000
ENV NEXT_TELEMETRY_DISABLED 1

CMD ["npm", "start"]