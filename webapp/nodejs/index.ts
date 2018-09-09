import TraceError from "trace-error";
import createFastify, { FastifyRequest } from "fastify";
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
  }

  interface FastifyRequest<HttpRequest> {
    session: any;
    sessionStore: any;
    user: any;
    administrator: any;
  }

  interface FastifyReply<HttpResponse> {
    view(name: string, params: object): void;
  }
}

interface LoginUser {
  id: number;
  nickname: string;
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

async function getConnection() {
  return fastify.mysql.getConnection();
}

async function getLoginUser<T>(request: FastifyRequest<T>): Promise<LoginUser | null> {
  const userId = request.session.user_id;

  if (!userId) {
    return Promise.resolve(null);
  } else {
    const conn = await getConnection();
    const [[row]] = await conn.query("SELECT id, nickname FROM users WHERE id = ?", [userId]);
    return { ...row };
  }
}

async function loginRequired(request, reply, done) {
  const user = await getLoginUser(request);
  if (!user) {
    resError(reply, "login_required", 401);
  }

  done();
}

async function fillinUser(request, _reply, done) {
  const user = await getLoginUser(request);
  if (user) {
    request.decorate("user", user);
  }

  done();
}

function resError(reply, error: string = "unknown", status: number = 500) {
  reply
    .type("application/json")
    .code(status)
    .send({ error });
}

type Event = any;

async function getEvents(where: (event: Event) => boolean = (eventRow) => eventRow.public_fg === 1): Promise<ReadonlyArray<Event>> {
  const conn = await getConnection();

  const events = [] as Array<Event>;

  await conn.beginTransaction();
  try {
    const [rows] = await conn.query("SELECT * FROM events ORDER BY id ASC");

    const eventIds = rows.filter((row) => where(row)).map((row) => row.id);

    for (const eventId of eventIds) {
      const event = await getEvent(eventId);

      for (const k of Object.keys(event!.sheets)) {
        delete event.sheets[k].detail;
      }

      events.push(event);
    }

    await conn.commit();
  } catch (e) {
    console.error(new TraceError("Failed to getEvents()", e));
    await conn.rollback();
  }

  console.log("events", events); // FIXME: remove this
  return events;
}

async function getEvent(eventId: number, loginUserId?: number): Promise<Event | null> {
  const conn = await getConnection();

  const [[eventRow]] = await conn.query("SELECT * FROM events WHERE id = ?", [eventId]);
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
    const sheetsForRank = event.sheets[rank] ? event.sheets[rank] : (event.sheets[rank] = { detail: [] });
    sheetsForRank.total = 0;
    sheetsForRank.remains = 0;
  }

  const [sheetRows] = await conn.query("SELECT * FROM sheets ORDER BY `rank`, num");

  for (const sheetRow of sheetRows) {
    const sheet = { ...sheetRow };
    if (!event.sheets[sheet.rank].price) {
      event.sheets[sheet.rank].price = event.price + sheet.price;
    }

    event.total++;
    event.sheets[sheet.rank].total++;

    const reservation = await conn.query("SELECT * FROM reservations WHERE event_id = ? AND sheet_id = ? AND canceled_at IS NULL GROUP BY event_id, sheet_id HAVING reserved_at = MIN(reserved_at)", [event.id, sheet.id]);
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

  event.public = !!event.public_fg;
  delete event.public_fg;
  event.closed = !!event.closed_fg;
  delete event.closed_fg;

  return event;
}

function sanitizeEvent(event: Event) {
  const sanitized = { ...event };
  delete sanitized.price;
  delete sanitized.public;
  delete sanitized.closed;
  return sanitized;
}

async function validateRank(rank: string): Promise<boolean> {
  const conn = await getConnection();
  const [[row]] = await conn.query("SELECT COUNT(*) FROM sheets WHERE `rank` = ?", [rank]);
  const [count] = Object.values(row);
  return count > 0;
}

function parseTimestampToEpoch(timestamp: string) {
  return Math.floor(new Date(timestamp).getTime() / 1000);
}

fastify.get("/", { beforeHandler: fillinUser }, async (request, reply) => {
  const events = (await getEvents()).map((event) => sanitizeEvent(event));

  reply.view("index.html.ejs", {
    uriFor: (path) => path,
    user: request.user,
    events
  });
});

fastify.get("/initialize", async (_request, reply) => {
  const conn = await getConnection();

  await conn.beginTransaction();
  try {
    await conn.query("DELETE FROM users WHERE id > 1000");
    await conn.query("DELETE FROM reservations WHERE id > 1000");
    await conn.query("UPDATE reservations SET canceled_at = NULL");
    await conn.query("DELETE FROM events WHERE id > 3");
    await conn.query("UPDATE events SET public_fg = 0, closed_fg = 1");
    await conn.query("UPDATE events SET public_fg = 1, closed_fg = 0 WHERE id = 1");
    await conn.query("UPDATE events SET public_fg = 1, closed_fg = 0 WHERE id = 2");
    await conn.query("UPDATE events SET public_fg = 0, closed_fg = 0 WHERE id = 3");

    await conn.commit();
  } catch (e) {
    console.error(new TraceError("Unexpected error", e));
    await conn.rollback();
  }

  reply.code(204);
});

fastify.get("/api/ping", async (_request, reply) => {
  reply.send({
    ping: !!(await getConnection()).ping()
  });
});

fastify.post("/api/users", {}, async (request, reply) => {
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
      const [result] = await conn.query("INSERT INTO users (login_name, pass_hash, nickname) VALUES (?, SHA2(?, 256), ?)", [loginName, password, nickname]);
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

  reply.code(201).send({
    id: userId,
    nickname
  });
});

fastify.get("/api/users/:id", { beforeHandler: loginRequired }, async (request, reply) => {
  const conn = await getConnection();
  const [[user]] = await conn.query("SELECT id, nickname FROM users WHERE id = ?", [Number.parseInt(request.params.id, 10)]);
  if (user.id !== (await getLoginUser(request))!.id) {
    return resError(reply, "forbidden", 403);
  }

  const recentReservations: Array<any> = [];
  {
    const [rows] = await conn.query("SELECT r.*, s.rank AS sheet_rank, s.num AS sheet_num FROM reservations r INNER JOIN sheets s ON s.id = r.sheet_id WHERE r.user_id = ? ORDER BY IFNULL(r.canceled_at, r.reserved_at) DESC LIMIT 5", [[user.id]]);

    for (const row of rows) {
      const event = await getEvent(row.event_id);

      const reservation = {
        id: row.id,
        event,
        sheet_rank: row.seet_rank,
        sheet_num: row.sheet_num,
        price: event.sheets[row.sheet_rank].price,
        reserved_at: parseTimestampToEpoch(row.reserved_at),
        canceled_at: row.canceled_at ? parseTimestampToEpoch(row.canceled_at) : null
      };

      delete event.sheets;
      delete event.total;
      delete event.remains;

      recentReservations.push(reservation);
    }
  }

  user.recent_reservations = recentReservations;
  user.total_price = await conn.query("SELECT IFNULL(SUM(e.price + s.price), 0) FROM reservations r INNER JOIN sheets s ON s.id = r.sheet_id INNER JOIN events e ON e.id = r.event_id WHERE r.user_id = ? AND r.canceled_at IS NULL", user.id);

  const recentEvents: Array<any> = [];
  {
    const [rows] = await conn.query("SELECT DISTINCT event_id FROM reservations WHERE user_id = ? ORDER BY IFNULL(canceled_at, reserved_at) DESC LIMIT 5", [user.id]);
    for (const row of rows) {
      const event = await getEvent(row.event_id);
      for (const sheetRank of Object.keys(event.sheets)) {
        delete event.sheets[sheetRank].detail;
        recentEvents.push(event);
      }
    }
  }
  user.recent_events = recentEvents;
  reply.send(user);
});

fastify.post("/api/actions/login", async (request, reply) => {
  const loginName = request.body.login_name;
  const password = request.body.password;

  const conn = await getConnection();
  const [[userRow]] = await conn.select("SELECT * FROM users WHERE login_name = ?", [loginName]);
  const [[passHash]] = await conn.select("SELECT SHA2(?, 256)", [password]);
  if (!userRow || passHash !== userRow.pass_hash) {
    return resError(reply, "authentication_failed", 401);
  }

  request.session.user_id = userRow.id;
  const user = await getLoginUser(request);
  reply.send(user);
});

fastify.post("/api/actions/logout", async (request, reply) => {
  const session = request.session;
  request.sessionStore.destroy(session.sessionId, () => {
    reply.code(204);
  });
});

fastify.get("/api/events", async (_request, reply) => {
  const events = (await getEvents()).map((event) => sanitizeEvent(event));
  reply.send(events);
});

fastify.get("/api/event/:id", async (request, reply) => {
  const eventId = request.params.id;
  const user = await getLoginUser(request);
  const event = await getEvent(eventId, user ? user.id : undefined);

  if (!event || !event.public) {
    return resError(reply, "not_found", 404);
  }

  const sanitizedEvent = sanitizeEvent(event);
  reply.send(sanitizedEvent);
});

fastify.post("/api/events/:id/actions/reserve", { beforeHandler: loginRequired }, async (request, reply) => {
  const conn = await getConnection();

  const eventId = request.params.id;
  const rank = request.body.sheet_rank;

  const user = (await getLoginUser(request))!;
  const event = await getEvent(eventId, user.id);
  if (!(event && event.public)) {
    return resError(reply, "invalid_event", 404);
  }
  if (!validateRank(rank)) {
    return resError(reply, "invalid_rank", 400);
  }

  let sheet: any;
  let reservationId: any;

  while (true) {
    sheet = await conn.query("SELECT * FROM sheets WHERE id NOT IN (SELECT sheet_id FROM reservations WHERE event_id = ? AND canceled_at IS NULL FOR UPDATE) AND `rank` = ? ORDER BY RAND() LIMIT 1", [event.id, rank]);

    if (!sheet) {
      return resError(reply, "sold_out", 409);
    }

    await conn.beginTransaction();
    try {
      const [result] = await conn.query("INSERT INTO reservations (event_id, sheet_id, user_id, reserved_at) VALUES (?, ?, ?, ?)", [event.id, sheet.id, user.id, new Date()]);
      reservationId = result.insertId;
      await conn.commit();
    } catch (e) {
      await conn.rollback();
      console.warn("re-try: rollback by:", e);
      continue;
    }

    break;
  }

  reply.code(202).send({
    reservation_id: reservationId,
    sheet_rank: rank,
    sheet_num: sheet.num
  });
});

fastify.delete("/api/events/:id/sheets/:rank/:num/reservation", { beforeHandler: loginRequired }, async (request, reply) => {
  const conn = await getConnection();

  const eventId = request.params.id;
  const rank = request.params.rank;
  const num = request.params.num;

  const user = (await getLoginUser(request))!;
  const event = await getEvent(eventId, user.id);
  if (!(event && event.public)) {
    return resError(reply, "invalid_event", 404);
  }
  if (!validateRank(rank)) {
    return resError(reply, "invalid_rank", 404);
  }

  const [[sheetRow]] = await conn.query("SELECT * FROM sheets WHERE `rank` = ? AND num = ?", [rank, num]);
  if (!sheetRow) {
    return resError(reply, "invalid_sheet", 404);
  }

  let done = false;
  await conn.beginTransaction();
  TRANSACTION: try {
    const [[reservation]] = await conn.query("SELECT * FROM reservations WHERE event_id = ? AND sheet_id = ? AND canceled_at IS NULL GROUP BY event_id HAVING reserved_at = MIN(reserved_at) FOR UPDATE", [event.id, sheetRow.id]);
    if (!reservation) {
      done = true;
      await conn.rollback();
      break TRANSACTION;
    }
    if (reservation.user_id !== user.id) {
      resError(reply, "not_permitted", 403);
      await conn.rollback();
      break TRANSACTION;
    }

    await conn.query("UPDATE reservations SET canceled_at = ? WHERE id = ?", [new Date(), reservation.id]);

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

  reply.code(204);
});

async function getLoginAdministrator<T>(request: FastifyRequest<T>): Promise<{ id; nickname } | null> {
  const administratorId = request.session.administrator_id;
  if (!administratorId) {
    return Promise.resolve(null);
  }

  const conn = await getConnection();
  const [[row]] = await conn.query("SELECT id, nickname FROM administrators WHERE id = ?", [administratorId]);
  return { ...row };
}

async function adminLoginRequired(request, reply, done) {
  const administrator = await getLoginAdministrator(request);
  if (!administrator) {
    resError(reply, "admin_login_required", 401);
  }
  done();
}

async function fillinAdministrator(request, _reply, done) {
  const administrator = await getLoginAdministrator(request);
  if (administrator) {
    request.decorate("administrator", administrator);
  }

  done();
}

fastify.get("/admin/", { beforeHandler: fillinAdministrator }, async (request, reply) => {
  let events: ReadonlyArray<any> | null = null;
  if (request.administrator) {
    events = await getEvents((event) => true);
  }

  reply.view("admin.html.ejs", {
    events,
    uriFor: (path) => path
  });
});

fastify.post("/admin/api/actions/login", async (request, reply) => {
  reply.code(500);
});

fastify.post("/admin/api/actions/logout", { beforeHandler: adminLoginRequired }, async (request, reply) => {
  reply.code(500);
});

fastify.get("/admin/api/events", { beforeHandler: adminLoginRequired }, async (request, reply) => {
  reply.code(500);
});

fastify.get("/admin/api/events/:id", { beforeHandler: adminLoginRequired }, async (request, reply) => {
  reply.code(500);
});

fastify.post("/admin/api/events/:id/actions/edit", { beforeHandler: adminLoginRequired }, async (request, reply) => {
  reply.code(500);
});

fastify.get("/admin/api/events/:id/sales", { beforeHandler: adminLoginRequired }, async (request, reply) => {
  reply.code(500);
});

fastify.post("/admin/api/reports/sales", { beforeHandler: adminLoginRequired }, async (request, reply) => {
  reply.code(500);
});

async function renderReportCsv(request, reply) {

}


fastify.listen(8080, (err, address) => {
  if (err) {
    throw new TraceError("Failed to listening", err);
  }
  fastify.log.info(`server listening on ${address}`);
});
