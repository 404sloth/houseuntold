import { MockApi } from './mockApi';
import type { VenuApiProvider } from './provider';
// To connect to the live Go backend, import RealApi and swap the single line below:
// import { RealApi } from './realApi';

export const api: VenuApiProvider = new MockApi();
export type { VenuApiProvider } from './provider';
