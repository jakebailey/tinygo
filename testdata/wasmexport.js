const fs = require('fs')

require('../targets/wasm_exec.js');

const wasm = fs.readFileSync(process.argv[2]);
const wasmModule = new WebAssembly.Module(wasm);
const jspi = WebAssembly.Module.exports(wasmModule).some((entry) => entry.name === 'tinygo_jspi_run');

function runTests() {
    const tests = [
        ['hello', [], undefined],
        ['add', [3, 5], 8],
        ['add', [7, 9], 16],
        ['add', [6, 1], 7],
        ['reentrantCall', [2, 3], 5],
        ['reentrantCall', [1, 8], 9],
        ['goroutineExit', [], undefined],
    ];
    const checkResult = (name, params, expected, result) => {
        if (result !== expected) {
            console.error(`${name}(...${params}): expected result ${expected}, got ${result}`);
        }
    };
    if (jspi) {
        return (async () => {
            for (const [name, params, expected] of tests) {
                const fn = WebAssembly.promising(go._inst.exports[name]);
                checkResult(name, params, expected, await fn(...params));
            }
        })();
    }

    for (const [name, params, expected] of tests) {
        const result = go._inst.exports[name](...params);
        checkResult(name, params, expected, result);
    }
}

let go = new Go();
const callOutside = (a, b) => {
    if (jspi) {
        return WebAssembly.promising(go._inst.exports.add)(a, b);
    }
    return go._inst.exports.add(a, b);
};
const callTestMain = () => runTests();
go.importObject.tester = {
    callOutside: jspi ? new WebAssembly.Suspending(callOutside) : callOutside,
    callTestMain: jspi ? new WebAssembly.Suspending(callTestMain) : callTestMain,
};
WebAssembly.instantiate(wasm, go.importObject).then(async (result) => {
    let buildMode = process.argv[3];
    if (buildMode === 'default') {
        await go.run(result.instance);
    } else if (buildMode === 'c-shared') {
        await go.run(result.instance);
        await runTests();
    }
}).catch((err) => {
    console.error(err);
    process.exit(1);
});
