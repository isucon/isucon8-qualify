import "source-map-support/register";
import TraceError from "trace-error";
import createFastify from "fastify";

const fastify = createFastify({
  logger: true,
});

fastify.get("/", (request, reply) => {
  reply.type("text/plain")
    .code(200)
    .send("Hello, torb.nodejs!");
});

fastify.listen(8080, (err, address) => {
  if (err) {
    throw new TraceError("Failed to listening", err);
  }
  fastify.log.info(`server listening on ${address}`);
});

