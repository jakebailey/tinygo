const fs = require('fs')

require('../targets/wasm_exec.js');

var callback;

global.setCallback = (cb) => {
    callback = cb;
    Promise.resolve().then(async () => {
        console.log('calling callback!');
        let result = await callback(1, 2, 3, 4);
        console.log('result from callback:', result);
    });
};

let go = new Go();
WebAssembly.instantiate(fs.readFileSync(process.argv[2]), go.importObject).then(async (result) => {
    await go.run(result.instance);
}).catch((err) => {
    console.error(err);
    process.exit(1);
});
