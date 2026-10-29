// The compiled client uses React/Ant Design production style hashes. External
// SSR dependencies must select the same mode before they are imported.
process.env.NODE_ENV='production'
await import('../dist/server/entry.mjs')
