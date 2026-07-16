"use strict";

if (process.argv.length < 3) {
    console.error("usage: wasm_gc_exec_node [wasm binary]");
    process.exit(1);
}

if (typeof WebAssembly.Suspending !== "function" ||
        typeof WebAssembly.promising !== "function") {
    console.error("WebAssembly JSPI is unavailable");
    process.exit(1);
}

const fs = require("fs");
const channels = new Map();
const goroutines = new Set();
let nextChannelID = 1;
let instance;
let goroutineFailure;

function makeChan(capacity) {
    const id = nextChannelID++;
    channels.set(id, {
        capacity,
        queue: [],
        senders: [],
        receivers: [],
    });
    return id;
}

function channel(id) {
    const value = channels.get(id);
    if (!value) {
        throw new Error(`invalid channel ${id}`);
    }
    return value;
}

async function channelSend(id, value) {
    const ch = channel(id);
    const receiver = ch.receivers.shift();
    if (receiver) {
        receiver(value);
        return 0;
    }
    if (ch.queue.length < ch.capacity) {
        ch.queue.push(value);
        return 0;
    }
    await new Promise((resolve) => {
        ch.senders.push({value, resolve});
    });
    return 0;
}

async function channelRecv(id) {
    const ch = channel(id);
    if (ch.queue.length !== 0) {
        const value = ch.queue.shift();
        const sender = ch.senders.shift();
        if (sender) {
            ch.queue.push(sender.value);
            sender.resolve();
        }
        return value;
    }
    const sender = ch.senders.shift();
    if (sender) {
        sender.resolve();
        return sender.value;
    }
    return new Promise((resolve) => {
        ch.receivers.push(resolve);
    });
}

const environment = {
    printInt(value) {
        console.log(value);
    },
    suspend: new WebAssembly.Suspending(async () => {
        await new Promise((resolve) => setImmediate(resolve));
        if (global.gc) {
            global.gc();
        }
        return 0;
    }),
    makeChan,
    channelSend: new WebAssembly.Suspending(channelSend),
    channelRecv: new WebAssembly.Suspending(channelRecv),
};

const imports = {
    env: new Proxy(environment, {
        get(target, name) {
            if (name in target) {
                return target[name];
            }
            const match = /^spawn(\d+)$/.exec(name);
            if (!match) {
                return undefined;
            }
            return (...args) => {
                const entry = instance.exports[`goroutine${match[1]}`];
                const promise = WebAssembly.promising(entry)(...args);
                goroutines.add(promise);
                promise.catch((err) => {
                    goroutineFailure ??= err;
                }).finally(() => goroutines.delete(promise));
            };
        },
    }),
};

WebAssembly.instantiate(fs.readFileSync(process.argv[2]), imports).then(async (result) => {
    instance = result.instance;
    const run = WebAssembly.promising(instance.exports.run);
    process.exitCode = await run();
    while (goroutines.size !== 0) {
        await Promise.allSettled([...goroutines]);
    }
    if (goroutineFailure) {
        throw goroutineFailure;
    }
}).catch((err) => {
    console.error(err);
    process.exit(1);
});
