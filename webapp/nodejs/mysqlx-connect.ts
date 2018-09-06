w
import mysqlx from "@mysql/xdevapi";

(async () => {
  const session = await mysqlx.getSession({
    host: process.env.DB_HOST,
    port: 33060, // process.env.DB_PORT,
    user: process.env.DB_USER,
    password: process.env.DB_PASS,
    schema: process.env.DB_DATABASE,
  });
  console.log('session created', session);
})();
