
declare module "trace-error" {
  export default class TraceError {
    constructor(message: string, ...causes: Array<Error>);
  }
}
