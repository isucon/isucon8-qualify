import "core-js";
import TraceError from "trace-error";
import createFastify, { FastifyRequest, FastifyReply } from "fastify";
import fastifyMysql from "fastify-mysql";
import fastifyCookie from "fastify-cookie";
import fastifyStatic from "fastify-static";
import pointOfView from "point-of-view";
import ejs from "ejs";
import path from "path";
import child_process from "child_process";
import util from "util";
import { IncomingMessage } from "http";

const execFile = util.promisify(child_process.execFile);

type MySQLResultRows = Array<any> & { insertId: number };
type MySQLColumnCatalogs = Array<any>;

type MySQLResultSet = [MySQLResultRows, MySQLColumnCatalogs];

interface MySQLQueryable {
  query(sql: string, params?: ReadonlyArray<any>): Promise<MySQLResultSet>;
}

interface MySQLClient extends MySQLQueryable {
  beginTransaction(): Promise<void>;
  commit(): Promise<void>;
  rollback(): Promise<void>;
  release(): void;
}

declare module "fastify" {
  interface FastifyInstance<HttpServer, HttpRequest, HttpResponse> {
    mysql: MySQLQueryable & {
      getConnection(): Promise<MySQLClient>;
    };
  }

  interface FastifyRequest<HttpRequest> {
    user: any;
    administrator: any;
    cookies: any;
  }

  interface FastifyReply<HttpResponse> {
    view(name: string, params: object): void;
    setCookie(name: string, value: string, opts: object): void;
  }
}

interface LoginUser {
  id: number;
  nickname: string;
}

const SECRET = "tagomoris";

const fastify = createFastify({
  logger: true,
});

fastify.register(fastifyStatic, {
  root: path.join(__dirname, "public"),
});

fastify.register(fastifyCookie);

fastify.register(pointOfView, {
  engine: { ejs },
  templates: path.join(__dirname, "templates"),
});

fastify.register(fastifyMysql, {
  host: process.env.DB_HOST,
  port: process.env.DB_PORT,
  user: process.env.DB_USER,
  password: process.env.DB_PASS,
  database: process.env.DB_DATABASE,

  promise: true,
});

fastify.decorateRequest("user", null);
fastify.decorateRequest("administrator", null);

function buildUriFor<T extends IncomingMessage>(request: FastifyRequest<T>) {
  const uriBase = `http://${request.headers.host}`;
  return (path) => {
    return `${uriBase}${path}`;
  };
}

async function getConnection() {
  return fastify.mysql.getConnection();
}

async function getLoginUser<T>(request: FastifyRequest<T>): Promise<LoginUser | null> {
  const userId = JSON.parse(request.cookies.user_id || "null");
  if (!userId) {
    return Promise.resolve(null);
  } else {
    const [[row]] = await fastify.mysql.query("SELECT id, nickname FROM users WHERE id = ?", [userId]);
    return { ...row };
  }
}

// NOTE: beforeHandler must not be async function
function loginRequired(request, reply, done) {
  getLoginUser(request).then((user) => {
    if (!user) {
      resError(reply, "login_required", 401);
    }
    done();
  });
}

function fillinUser(request, _reply, done) {
  getLoginUser(request).then((user) => {
    if (user) {
      request.user = user;
    }
    done();
  });
}

type Event = any;

async function getEvents(where: (event: Event) => boolean = (eventRow) => !!eventRow.public_fg): Promise<ReadonlyArray<Event>> {
  const conn = await getConnection();

  const events = [] as Array<Event>;

  await conn.beginTransaction();
  try {
    const [rows] = await conn.query("SELECT * FROM events ORDER BY id ASC");

    const eventIds = rows.filter((row) => where(row)).map((row) => row.id);

    for (const eventId of eventIds) {
      const event = (await getEvent(eventId))!;

      for (const rank of Object.keys(event.sheets)) {
        delete event.sheets[rank].detail;
      }

      events.push(event);
    }

    await conn.commit();
  } catch (e) {
    console.error(new TraceError("Failed to getEvents()", e));
    await conn.rollback();
  }

  await conn.release();
  return events;
}

async function getEvent(eventId: number, loginUserId?: number): Promise<Event | null> {
  const [[eventRow]] = await fastify.mysql.query("SELECT * FROM events WHERE id = ?", [eventId]);
  if (!eventRow) {
    return null;
  }

  const event = {
    ...eventRow,
    sheets: {},
  };

  // zero fill
  event.total = 0;
  event.remains = 0;
  for (const rank of ["S", "A", "B", "C"]) {
    const sheetsForRank = event.sheets[rank] ? event.sheets[rank] : (event.sheets[rank] = { detail: [] });
    sheetsForRank.total = 0;
    sheetsForRank.remains = 0;
  }

  const [sheetRows] = await fastify.mysql.query("SELECT * FROM sheets ORDER BY `rank`, num");

  for (const sheetRow of sheetRows) {
    const sheet = { ...sheetRow };
    if (!event.sheets[sheet.rank].price) {
      event.sheets[sheet.rank].price = event.price + sheet.price;
    }

    event.total++;
    event.sheets[sheet.rank].total++;

    const [[reservation]] = await fastify.mysql.query("SELECT * FROM reservations WHERE event_id = ? AND sheet_id = ? AND canceled_at IS NULL GROUP BY event_id, sheet_id HAVING reserved_at = MIN(reserved_at)", [event.id, sheet.id]);
    if (reservation) {
      if (loginUserId && reservation.user_id === loginUserId) {
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
  const [[row]] = await fastify.mysql.query("SELECT COUNT(*) FROM sheets WHERE `rank` = ?", [rank]);
  const [count] = Object.values(row);
  return count > 0;
}

function parseTimestampToEpoch(timestamp: string) {
  return Math.floor(new Date(timestamp+"Z").getTime() / 1000);
}

fastify.get("/", { beforeHandler: fillinUser }, async (request, reply) => {
  const events = (await getEvents()).map((event) => sanitizeEvent(event));

  reply.view("index.html.ejs", {
    uriFor: buildUriFor(request),
    user: request.user,
    events,
  });
});

fastify.get("/initialize", async (_request, reply) => {
  await execFile("../../db/init.sh");

  reply.code(204);
});

fastify.post("/api/users", async (request, reply) => {
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

  conn.release();

  if (done) {
    return;
  }

  reply.code(201).send({
    id: userId,
    nickname,
  });
});

fastify.get("/api/users/:id", { beforeHandler: loginRequired }, async (request, reply) => {
  const [[user]] = await fastify.mysql.query("SELECT id, nickname FROM users WHERE id = ?", [request.params.id]);
  if (user.id !== (await getLoginUser(request))!.id) {
    return resError(reply, "forbidden", 403);
  }

  const recentReservations: Array<any> = [];
  {
    const [rows] = await fastify.mysql.query("SELECT r.*, s.rank AS sheet_rank, s.num AS sheet_num FROM reservations r INNER JOIN sheets s ON s.id = r.sheet_id WHERE r.user_id = ? ORDER BY IFNULL(r.canceled_at, r.reserved_at) DESC LIMIT 5", [[user.id]]);

    for (const row of rows) {
      const event = await getEvent(row.event_id);

      const reservation = {
        id: row.id,
        event,
        sheet_rank: row.sheet_rank,
        sheet_num: row.sheet_num,
        price: event.sheets[row.sheet_rank].price,
        reserved_at: parseTimestampToEpoch(row.reserved_at),
        canceled_at: row.canceled_at ? parseTimestampToEpoch(row.canceled_at) : null,
      };

      delete event.sheets;
      delete event.total;
      delete event.remains;

      recentReservations.push(reservation);
    }
  }

  user.recent_reservations = recentReservations;

  const [[totalPriceRow]] = await fastify.mysql.query("SELECT IFNULL(SUM(e.price + s.price), 0) FROM reservations r INNER JOIN sheets s ON s.id = r.sheet_id INNER JOIN events e ON e.id = r.event_id WHERE r.user_id = ? AND r.canceled_at IS NULL", user.id);
  const [totalPriceStr] = Object.values(totalPriceRow);
  user.total_price = Number.parseInt(totalPriceStr, 10);

  const recentEvents: Array<any> = [];
  {
    const [rows] = await fastify.mysql.query("SELECT event_id FROM reservations WHERE user_id = ? GROUP BY event_id ORDER BY MAX(IFNULL(canceled_at, reserved_at)) DESC LIMIT 5", [user.id]);
    for (const row of rows) {
      const event = await getEvent(row.event_id);
      for (const sheetRank of Object.keys(event.sheets)) {
        delete event.sheets[sheetRank].detail;
      }
      recentEvents.push(event);
    }
  }
  user.recent_events = recentEvents;
  reply.send(user);
});

fastify.post("/api/actions/login", async (request, reply) => {
  const loginName = request.body.login_name;
  const password = request.body.password;

  const [[userRow]] = await fastify.mysql.query("SELECT * FROM users WHERE login_name = ?", [loginName]);
  const [[passHashRow]] = await fastify.mysql.query("SELECT SHA2(?, 256)", [password]);
  const [passHash] = Object.values(passHashRow);
  if (!userRow || passHash !== userRow.pass_hash) {
    return resError(reply, "authentication_failed", 401);
  }

  reply.setCookie("user_id", userRow.id, {
    path: "/",
  });
  request.cookies.user_id = `${userRow.id}`; // for the follong getLoginUser()
  const user = await getLoginUser(request);
  reply.send(user);
});

fastify.post("/api/actions/logout", { beforeHandler: loginRequired }, async (_request, reply) => {
  reply.setCookie("user_id", "", {
    path: "/",
    expires: new Date(0),
  });
  reply.code(204);
});

fastify.get("/api/events", async (_request, reply) => {
  const events = (await getEvents()).map((event) => sanitizeEvent(event));
  reply.send(events);
});

fastify.get("/api/events/:id", async (request, reply) => {
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
  const eventId = request.params.id;
  const rank = request.body.sheet_rank;

  const user = (await getLoginUser(request))!;
  const event = await getEvent(eventId, user.id);
  if (!(event && event.public)) {
    return resError(reply, "invalid_event", 404);
  }

  if (!await validateRank(rank)) {
    return resError(reply, "invalid_rank", 400);
  }

  let sheetRow: any;
  let reservationId: any;
  while (true) {
    [[sheetRow]] = await fastify.mysql.query("SELECT * FROM sheets WHERE id NOT IN (SELECT sheet_id FROM reservations WHERE event_id = ? AND canceled_at IS NULL FOR UPDATE) AND `rank` = ? ORDER BY RAND() LIMIT 1", [event.id, rank]);

    if (!sheetRow) {
      return resError(reply, "sold_out", 409);
    }

    const conn = await getConnection();
    await conn.beginTransaction();
    try {
      const [result] = await conn.query("INSERT INTO reservations (event_id, sheet_id, user_id, reserved_at) VALUES (?, ?, ?, ?)", [event.id, sheetRow.id, user.id, new Date()]);
      reservationId = result.insertId;
      await conn.commit();
    } catch (e) {
      await conn.rollback();
      console.warn("re-try: rollback by:", e);
      continue; // retry
    } finally {
      conn.release();
    }
    break;
  }

  reply.code(202).send({
    id: reservationId,
    sheet_rank: rank,
    sheet_num: sheetRow.num,
  });
});

fastify.delete("/api/events/:id/sheets/:rank/:num/reservation", { beforeHandler: loginRequired }, async (request, reply) => {
  const eventId = request.params.id;
  const rank = request.params.rank;
  const num = request.params.num;

  const user = (await getLoginUser(request))!;
  const event = await getEvent(eventId, user.id);
  if (!(event && event.public)) {
    return resError(reply, "invalid_event", 404);
  }
  if (!await validateRank(rank)) {
    return resError(reply, "invalid_rank", 404);
  }

  const [[sheetRow]] = await fastify.mysql.query("SELECT * FROM sheets WHERE `rank` = ? AND num = ?", [rank, num]);
  if (!sheetRow) {
    return resError(reply, "invalid_sheet", 404);
  }

  const conn = await getConnection();
  let done = false;
  await conn.beginTransaction();
  TRANSACTION: try {
    const [[reservationRow]] = await conn.query("SELECT * FROM reservations WHERE event_id = ? AND sheet_id = ? AND canceled_at IS NULL GROUP BY event_id HAVING reserved_at = MIN(reserved_at) FOR UPDATE", [event.id, sheetRow.id]);
    if (!reservationRow) {
      resError(reply, "not_reserved", 400);
      done = true;
      await conn.rollback();
      break TRANSACTION;
    }
    if (reservationRow.user_id !== user.id) {
      resError(reply, "not_permitted", 403);
      done = true;
      await conn.rollback();
      break TRANSACTION;
    }

    await conn.query("UPDATE reservations SET canceled_at = ? WHERE id = ?", [new Date(), reservationRow.id]);

    await conn.commit();
  } catch (e) {
    console.warn("rollback by:", e);
    await conn.rollback();
    resError(reply);
    done = true;
  }

  conn.release();
  if (done) {
    return;
  }

  reply.code(204);
});

async function getLoginAdministrator<T>(request: FastifyRequest<T>): Promise<{ id; nickname } | null> {
  const administratorId = JSON.parse(request.cookies.administrator_id || "null");
  if (!administratorId) {
    return Promise.resolve(null);
  }
  const [[row]] = await fastify.mysql.query("SELECT id, nickname FROM administrators WHERE id = ?", [administratorId]);
  return { ...row };
}

function adminLoginRequired(request, reply, done) {
  getLoginAdministrator(request).then((administrator) => {
    if (!administrator) {
      resError(reply, "admin_login_required", 401);
    }
    done();
  });
}

function fillinAdministrator(request, _reply, done) {
  getLoginAdministrator(request).then((administrator) => {
    if (administrator) {
      request.administrator = administrator;
    }

    done();
  });
}

fastify.get("/admin/", { beforeHandler: fillinAdministrator }, async (request, reply) => {
  let events: ReadonlyArray<any> = [];
  if (request.administrator) {
    events = await getEvents((_event) => true);
  }

  reply.view("admin.html.ejs", {
    events,
    administrator: request.administrator,
    uriFor: buildUriFor(request),
  });
});

fastify.post("/admin/api/actions/login", async (request, reply) => {
  const loginName = request.body.login_name;
  const password = request.body.password;

  const [[administratorRow]] = await fastify.mysql.query("SELECT * FROM administrators WHERE login_name = ?", [loginName]);
  const [[passHashRow]] = await fastify.mysql.query("SELECT SHA2(?, 256)", [password]);
  const [passHash] = Object.values(passHashRow);
  if (!administratorRow || passHash !== administratorRow.pass_hash) {
    return resError(reply, "authentication_failed", 401);
  }
  reply.setCookie("administrator_id", administratorRow.id, {
    path: "/",
  });
  request.cookies.administrator_id = `${administratorRow.id}`; // for the follong getLoginAdministratorUser()
  const administrator = await getLoginAdministrator(request);

  reply.send(administrator);
});

fastify.post("/admin/api/actions/logout", { beforeHandler: adminLoginRequired }, async (request, reply) => {
  reply.setCookie("administrator_id", "", {
    path: "/",
    expires: new Date(0),
  });
  reply.code(204);
});

fastify.get("/admin/api/events", { beforeHandler: adminLoginRequired }, async (_request, reply) => {
  const events = await getEvents((_) => true);
  reply.send(events);
});

fastify.post("/admin/api/events", { beforeHandler: adminLoginRequired }, async (request, reply) => {
  const title = request.body.title;
  const isPublic = request.body.public;
  const price = request.body.price;

  let eventId: number | null = null;

  const conn = await getConnection();
  await conn.beginTransaction();
  try {
    const [result] = await conn.query("INSERT INTO events (title, public_fg, closed_fg, price) VALUES (?, ?, 0, ?)", [title, isPublic, price]);
    eventId = result.insertId;
    await conn.commit();
  } catch (e) {
    console.error(e);
    await conn.rollback();
  }
  conn.release();

  const event = await getEvent(eventId!);
  reply.send(event);
});

fastify.get("/admin/api/events/:id", { beforeHandler: adminLoginRequired }, async (request, reply) => {
  const eventId = request.params.id;
  const event = await getEvent(eventId);
  if (!event) {
    return resError(reply, "not_found", 404);
  }
  reply.send(event);
});

fastify.post("/admin/api/events/:id/actions/edit", { beforeHandler: adminLoginRequired }, async (request, reply) => {
  const eventId = request.params.id;
  const closed = request.body.closed;
  const isPublic = closed ? false : !!request.body.public;

  const event = await getEvent(eventId);
  if (!event) {
    return resError(reply, "not_found", 404);
  }

  if (event.closed) {
    return resError(reply, "cannot_edit_closed_event", 400);
  } else if (event.public && closed) {
    return resError(reply, "cannot_edit_closed_event", 400);
  }

  const conn = await getConnection();
  await conn.beginTransaction();
  try {
    await conn.query("UPDATE events SET public_fg = ?, closed_fg = ? WHERE id = ?", [isPublic, closed, event.id]);
    await conn.commit();
  } catch (e) {
    console.error(e);
    await conn.rollback();
  }
  conn.release();

  const updatedEvent = await getEvent(eventId);
  reply.send(updatedEvent);
});

fastify.get("/admin/api/reports/events/:id/sales", { beforeHandler: adminLoginRequired }, async (request, reply) => {
  const eventId = request.params.id;
  const event = await getEvent(eventId);

  let reports: Array<any> = [];

  const [reservationRows] = await fastify.mysql.query("SELECT r.*, s.rank AS sheet_rank, s.num AS sheet_num, s.price AS sheet_price, e.price AS event_price FROM reservations r INNER JOIN sheets s ON s.id = r.sheet_id INNER JOIN events e ON e.id = r.event_id WHERE r.event_id = ? ORDER BY reserved_at ASC FOR UPDATE", [eventId]);
  for (const reservationRow of reservationRows) {
    const report = {
      reservation_id: reservationRow.id,
      event_id: event.id,
      rank: reservationRow.sheet_rank,
      num: reservationRow.sheet_num,
      user_id: reservationRow.user_id,
      sold_at: new Date(reservationRow.reserved_at).toISOString(),
      canceled_at: reservationRow.canceled_at ? new Date(reservationRow.canceled_at).toISOString() : "",
      price: reservationRow.event_price + reservationRow.sheet_price,
    };

    reports.push(report);
  }

  renderReportCsv(reply, reports);
});

fastify.get("/admin/api/reports/sales", { beforeHandler: adminLoginRequired }, async (request, reply) => {
  let reports: Array<any> = [];

  const [reservationRows] = await fastify.mysql.query("SELECT r.*, s.rank AS sheet_rank, s.num AS sheet_num, s.price AS sheet_price, e.id AS event_id, e.price AS event_price FROM reservations r INNER JOIN sheets s ON s.id = r.sheet_id INNER JOIN events e ON e.id = r.event_id ORDER BY reserved_at ASC FOR UPDATE");
  for (const reservationRow of reservationRows) {
    const report = {
      reservation_id: reservationRow.id,
      event_id: reservationRow.event_id,
      rank: reservationRow.sheet_rank,
      num: reservationRow.sheet_num,
      user_id: reservationRow.user_id,
      sold_at: new Date(reservationRow.reserved_at).toISOString(),
      canceled_at: reservationRow.canceled_at ? new Date(reservationRow.canceled_at).toISOString() : "",
      price: reservationRow.event_price + reservationRow.sheet_price,
    };

    reports.push(report);
  }

  renderReportCsv(reply, reports);
});

async function renderReportCsv<T>(reply: FastifyReply<T>, reports: ReadonlyArray<any>) {
  const sortedReports = [...reports].sort((a, b) => {
    return a.sold_at.localeCompare(b.sold_at);
  });

  const keys = ["reservation_id", "event_id", "rank", "num", "price", "user_id", "sold_at", "canceled_at"];

  let body = keys.join(",");
  body += "\n";
  for (const report of sortedReports) {
    body += keys.map((key) => report[key]).join(",");
    body += "\n";
  }

  reply
    .headers({
      "Content-Type": "text/csv; charset=UTF-8",
      "Content-Disposition": 'attachment; filename="report.csv"',
    })
    .send(body);
}

function resError(reply, error: string = "unknown", status: number = 500) {
  reply
    .type("application/json")
    .code(status)
    .send({ error });
}

fastify.listen(8080, (err, address) => {
  if (err) {
    throw new TraceError("Failed to listening", err);
  }
  fastify.log.info(`server listening on ${address}`);
});
