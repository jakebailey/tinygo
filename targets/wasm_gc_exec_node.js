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
const stressGC = process.env.TINYGO_WASMGC_STRESS_GC === "1";
let nextChannelID = 1;
let instance;
let goroutineFailure;

function collectGarbage() {
    if (stressGC && global.gc) {
        global.gc();
    }
}

function trackGoroutine(promise) {
    goroutines.add(promise);
    promise.catch((err) => {
        goroutineFailure ??= err;
    }).finally(() => goroutines.delete(promise));
}

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
    collectGarbage();
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
    const value = await new Promise((resolve) => {
        ch.receivers.push(resolve);
    });
    collectGarbage();
    return value;
}

const environment = {
    printInt(value) {
        console.log(value);
    },
    makeChan,
    channelSend: new WebAssembly.Suspending(channelSend),
    channelRecv: new WebAssembly.Suspending(channelRecv),
    scheduleTask(site) {
        const promise = (async () => {
            await new Promise((resolve) => setImmediate(resolve));
            collectGarbage();
            const entry = WebAssembly.promising(instance.exports.runTask);
            await entry(site);
        })();
        trackGoroutine(promise);
    },
};

const imports = {
    env: environment,
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
