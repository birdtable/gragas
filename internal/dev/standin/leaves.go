package main

// leaves is the operation this build leaves out: none. The module the digest
// pins is this one. A test that wants a module without an operation changes
// this constant through go build's -overlay (configtest.StandinWasmWithout,
// gragas#2289), so no build tag and no second source exists.
const leaves = ""
