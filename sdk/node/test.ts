import { MuninnClient, MuninnError, MuninnAuthError, MuninnNotFoundError } from './src/index';

const client = new MuninnClient({ baseUrl: 'http://localhost:9999', token: 'test' });
console.log('Client created OK');

const err = new MuninnError('test', 400);
console.log(`Error class OK: ${err.message}`);

const authErr = new MuninnAuthError();
console.log(`AuthError class OK: ${authErr.statusCode === 401}`);

const notFound = new MuninnNotFoundError();
console.log(`NotFoundError class OK: ${notFound.statusCode === 404}`);

client.close();
console.log('All exports verified OK');
