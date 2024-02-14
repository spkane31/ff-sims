FROM rust:latest as rust_build_stage

RUN apt-get update && apt-get upgrade -y
RUN apt-get install curl

RUN curl https://rustwasm.github.io/wasm-pack/installer/init.sh -sSf | sh

RUN mkdir -p /build/client/wasm
ADD . /build/client/wasm
WORKDIR /build/client/wasm
RUN wasm-pack build

FROM node:20 as svelte_build_stage

RUN mkdir /build

ADD ./www /build/svelte-client
RUN ls /build/svelte-client
# RUN mkdir /build/svelte-client/node_modules
WORKDIR /build/svelte-client

COPY --from=rust_build_stage /build/client/wasm/pkg/ /build/wasm/pkg/

RUN npm install
RUN npm run build

FROM nginx:latest

COPY --from=svelte_build_stage /build/svelte-client/dist/ /usr/share/nginx/pong/web