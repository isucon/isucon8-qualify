import TraceError from "trace-error";
import createFastify from "fastify";
import fastifyPlugin from "fastify-plugin";
import fastifyMysql from "fastify-mysql";
import fastifyCookie from "fastify-cookie";
import fastifySession from "fastify-session";
import fastifyStatic from "fastify-static";
import pointOfView from "point-of-view";
import ejs from "ejs";
import path from "path";

declare module "fastify" {
  interface FastifyInstance<HttpServer, HttpRequest, HttpResponse> {
    mysql: any;

    user: any;
  }

  interface FastifyReply<HttpResponse> {
    view(name: string, params: object): void;
  }
}

const fastify = createFastify({
  logger: true
});

fastify.register(fastifyStatic, {
  root: path.join(__dirname, "public")
});

fastify.register(fastifyCookie);
fastify.register(fastifySession, {
  secret: "tagomoris" + ".".repeat(32)
});

fastify.register(pointOfView, {
  engine: { ejs },
  templates: path.join(__dirname, "templates")
});

fastify.register(fastifyMysql, {
  host: process.env.DB_HOST,
  port: process.env.DB_PORT,
  user: process.env.DB_USER,
  password: process.env.DB_PASS,
  database: process.env.DB_DATABASE,

  promise: true
});

fastify.register(
  fastifyPlugin(async (fastify, _options, next) => {
    const conn = await getConnection();
    const [[user]] = await conn.query("select * from users limit 1");
    fastify.decorate("user", { ...user });

    next();
  })
);

async function getConnection() {
  return fastify.mysql.getConnection();
}

async function getLoginUser(request) {
  const userId = request.session.user_id;

  if (!userId) {
    return null;
  } else {
    const conn = await getConnection();
    const [[row]] = await conn.query(
      "SELECT id, nickname FROM users WHERE id = ?",
      [userId]
    );
    return row;
  }
}

async function loginRequired(request, reply, done) {
  resError(reply, "login_required", 401);

  done();
}

function resError(reply, error: string = "unknown", status: number = 500) {
  reply
    .type("application/json")
    .code(status)
    .send({ error });
}

type Event = any;

async function getEvents(
  where: (event: Event) => boolean = eventRow => eventRow.public_fg === 1
): Promise<ReadonlyArray<Event>> {
  const conn = await getConnection();

  const events = [] as Array<Event>;

  await conn.beginTransaction();
  try {
    const [rows] = await conn.query("SELECT * FROM events ORDER BY id ASC");

    const eventIds = rows.filter(row => where(row)).map(row => row.id);

    for (const eventId of eventIds) {
      const event = await getEvent(eventId);
      console.log(event);

      for (const k of Object.keys(event!.sheets)) {
        delete event.sheets[k].detail;
      }

      events.push(event);
    }

    await conn.commit();
  } catch (e) {
    console.error(e);
    await conn.rollback();
  }

  console.log("events", events); // FIXME: remove this
  return events;
}

async function getEvent(eventId, loginUserId?): Promise<Event | null> {
  const conn = await getConnection();

  const [[eventRow]] = await conn.query("SELECT * FROM events WHERE id = ?", [
    eventId
  ]);
  if (!eventRow) {
    return null;
  }

  const event = {
    ...eventRow,
    sheets: {}
  };

  // zero fill
  event.total = 0;
  event.remains = 0;
  for (const rank of ["S", "A", "B", "C"]) {
    const sheetsForRank = event.sheets[rank]
      ? event.sheets[rank]
      : (event.sheets[rank] = { detail: [] });
    sheetsForRank.total = 0;
    sheetsForRank.remains = 0;
  }

  const [sheetRows] = await conn.query(
    "SELECT * FROM sheets ORDER BY `rank`, num"
  );

  for (const sheetRow of sheetRows) {
    const sheet = { ...sheetRow };
    if (!event.sheets[sheet.rank].price) {
      event.sheets[sheet.rank].price = event.price + sheet.price;
    }

    event.total++;
    event.sheets[sheet.rank].total++;

    const reservation = await conn.query(
      "SELECT * FROM reservations WHERE event_id = ? AND sheet_id = ? AND canceled_at IS NULL GROUP BY event_id, sheet_id HAVING reserved_at = MIN(reserved_at)",
      [event.id, sheet.id]
    );
    if (reservation) {
      if (loginUserId && reservation.userId === loginUserId) {
        sheet.mine = true;
      }

      sheet.reserved = true;
      sheet.reserved_at = parseTimestampToEpoch(reservation.reserved_at);
    } else {
      event.remains++;
      event.sheets[sheet.rank].remains++;
    }

    event.sheets[sheet.rank].detail.push(sheet);

    delete sheet.id;
    delete sheet.price;
    delete sheet.rank;
  }

  return event;
}

function sanitizeEvent(event: Event) {
  const sanitized = {...event};
  delete sanitized.price;
  delete sanitized.public;
  delete sanitized.closed;
  return sanitized;
}

function parseTimestampToEpoch(timestamp: string) {
  return Math.floor(new Date(timestamp).getTime() / 1000);
}

fastify.get("/", async (_request, reply) => {
  const events = (await getEvents()).map(event => sanitizeEvent(event));

  reply.view("index.html.ejs", {
    uriFor: path => path,
    user: fastify.user,
    events
  });
});

fastify.get('/initialize', async (_request, reply) => {
  const conn = await getConnection();

  await conn.beginTransaction();
  try {
    await conn.query('DELETE FROM users WHERE id > 1000');
    await conn.query('DELETE FROM reservations WHERE id > 1000');
    await conn.query('UPDATE reservations SET canceled_at = NULL');
    await conn.query('DELETE FROM events WHERE id > 3');
    await conn.query('UPDATE events SET public_fg = 0, closed_fg = 1');
    await conn.query('UPDATE events SET public_fg = 1, closed_fg = 0 WHERE id = 1');
    await conn.query('UPDATE events SET public_fg = 1, closed_fg = 0 WHERE id = 2');
    await conn.query('UPDATE events SET public_fg = 0, closed_fg = 0 WHERE id = 3');

    await conn.commit();
  } catch (e) {
    console.error(e);
    await conn.rollback();
  }

  reply.code(204);
});

fastify.post('/api/users', {}, async (request, reply) => {
  const nickname = request.body.nickname;
  const loginName = request.body.login_name;
  const password = request.body.password;

  const conn = await getConnection();

  let done = false;
  let userId;

  await conn.beginTransaction();
  try {
    const [[duplicatedRow]] = await conn.query("SELECT * FROM users WHERE login_name = ?", [loginName]);
    if (duplicatedRow) {
      resError(reply, "duplicated", 409);
      done = true;
      await conn.rollback();
    } else {
      const [result]= await conn.query("INSERT INTO users (login_name, pass_hash, nickname) VALUES (?, SHA2(?, 256), ?)", [loginName, password, nickname]);
      userId = result.insertId;
    }

    await conn.commit();
  } catch (e) {
    console.warn("rollback by:", e);

    await conn.rollback();
    resError(reply);
    done = true;
  }

  if (done) {
    return;
  }


  reply.code(201)
    .send({
      id: userId,
      nickname,
    });
});

fastify.get(
  "/api/users/:id",
  { beforeHandler: loginRequired },
  async (request, reply) => {
    console.log(request.params);

  }
);

fastify.listen(8080, (err, address) => {
  if (err) {
    throw new TraceError("Failed to listening", err);
  }
  fastify.log.info(`server listening on ${address}`);
});
