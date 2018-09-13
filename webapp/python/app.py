import MySQLdb.cursors
import flask
import functools
import os
import pathlib
import copy
import json
import subprocess
from io import StringIO
import csv
from datetime import datetime, timezone


base_path = pathlib.Path(__file__).resolve().parent.parent
static_folder = base_path / 'static'
icons_folder = base_path / 'public' / 'icons'


class CustomFlask(flask.Flask):
    jinja_options = flask.Flask.jinja_options.copy()
    jinja_options.update(dict(
        block_start_string='(%',
        block_end_string='%)',
        variable_start_string='((',
        variable_end_string='))',
        comment_start_string='(#',
        comment_end_string='#)',
    ))


app = CustomFlask(__name__, static_folder=str(static_folder), static_url_path='')
app.config['SECRET_KEY'] = 'tagomoris'


if not os.path.exists(str(icons_folder)):
    os.makedirs(str(icons_folder))


def make_base_url(request):
    return request.url_root[:-1]


@app.template_filter('tojsonsafe')
def tojsonsafe(target):
    return json.dumps(target).replace("+", "\\u002b").replace("<", "\\u003c").replace(">", "\\u003e")


def jsonify(target):
    return json.dumps(target)


def res_error(error="unknown", status=500):
    return (jsonify({"error": error}), status)


def login_required(f):
    @functools.wraps(f)
    def wrapper(*args, **kwargs):
        if not get_login_user():
            return res_error('login_required', 401)
        return f(*args, **kwargs)
    return wrapper


def admin_login_required(f):
    @functools.wraps(f)
    def wrapper(*args, **kwargs):
        if not get_login_administrator():
            return res_error('admin_login_required', 401)
        return f(*args, **kwargs)
    return wrapper


def dbh():
    if hasattr(flask.g, 'db'):
        return flask.g.db
    flask.g.db = MySQLdb.connect(
        host=os.environ['DB_HOST'],
        port=3306,
        user=os.environ['DB_USER'],
        password=os.environ['DB_PASS'],
        database=os.environ['DB_DATABASE'],
        charset='utf8mb4',
        cursorclass=MySQLdb.cursors.DictCursor,
        autocommit=True,
    )
    cur = flask.g.db.cursor()
    cur.execute("SET SESSION sql_mode='STRICT_TRANS_TABLES,NO_ZERO_IN_DATE,NO_ZERO_DATE,ERROR_FOR_DIVISION_BY_ZERO,NO_ENGINE_SUBSTITUTION'")
    return flask.g.db


@app.teardown_appcontext
def teardown(error):
    if hasattr(flask.g, "db"):
        flask.g.db.close()


def get_events(filter=lambda e: True):
    conn = dbh()
    conn.autocommit(False)
    cur = conn.cursor()
    try:
        cur.execute("SELECT * FROM events ORDER BY id ASC")
        rows = cur.fetchall()
        event_ids = [row['id'] for row in rows if filter(row)]
        events = []
        for event_id in event_ids:
            event = get_event(event_id)
            for sheet in event['sheets'].values():
                del sheet['detail']
            events.append(event)
        conn.commit()
    except MySQLdb.Error as e:
        conn.rollback()
        raise e
    return events


def get_event(event_id, login_user_id=None):
    cur = dbh().cursor()
    cur.execute("SELECT * FROM events WHERE id = %s", [event_id])
    event = cur.fetchone()
    if not event: return None

    event["total"] = 0
    event["remains"] = 0
    event["sheets"] = {}
    for rank in ["S", "A", "B", "C"]:
        event["sheets"][rank] = {'total': 0, 'remains': 0, 'detail': []}

    cur.execute("SELECT * FROM sheets ORDER BY `rank`, num")
    sheets = cur.fetchall()
    for sheet in sheets:
        if not event['sheets'][sheet['rank']].get('price'):
            event['sheets'][sheet['rank']]['price'] = event['price'] + sheet['price']
        event['total'] += 1
        event['sheets'][sheet['rank']]['total'] += 1

        cur.execute(
            "SELECT * FROM reservations WHERE event_id = %s AND sheet_id = %s AND canceled_at IS NULL GROUP BY event_id, sheet_id HAVING reserved_at = MIN(reserved_at)",
            [event['id'], sheet['id']])
        reservation = cur.fetchone()
        if reservation:
            if login_user_id and reservation['user_id'] == login_user_id:
                sheet['mine'] = True
            sheet['reserved'] = True
            sheet['reserved_at'] = int(reservation['reserved_at'].replace(tzinfo=timezone.utc).timestamp())
        else:
            event['remains'] += 1
            event['sheets'][sheet['rank']]['remains'] += 1

        event['sheets'][sheet['rank']]['detail'].append(sheet)

        del sheet['id']
        del sheet['price']
        del sheet['rank']

    event['public'] = True if event['public_fg'] else False
    event['closed'] = True if event['closed_fg'] else False
    del event['public_fg']
    del event['closed_fg']
    return event


def sanitize_event(event):
    sanitized = copy.copy(event)
    del sanitized['price']
    del sanitized['public']
    del sanitized['closed']
    return sanitized


def get_login_user():
    if "user_id" not in flask.session:
        return None
    cur = dbh().cursor()
    user_id = flask.session['user_id']
    cur.execute("SELECT id, nickname FROM users WHERE id = %s", [user_id])
    return cur.fetchone()


def get_login_administrator():
    if "administrator_id" not in flask.session:
        return None
    cur = dbh().cursor()
    administrator_id = flask.session['administrator_id']
    cur.execute("SELECT id, nickname FROM administrators WHERE id = %s", [administrator_id])
    return cur.fetchone()


def validate_rank(rank):
    cur = dbh().cursor()
    cur.execute("SELECT COUNT(*) AS total_sheets FROM sheets WHERE `rank` = %s", [rank])
    ret = cur.fetchone()
    return int(ret['total_sheets']) > 0


def render_report_csv(reports):
    reports = sorted(reports, key=lambda x: x['sold_at'])

    keys = ["reservation_id", "event_id", "rank", "num", "price", "user_id", "sold_at", "canceled_at"]

    body = []
    body.append(keys)
    for report in reports:
        body.append([report[key] for key in keys])

    f = StringIO()
    writer = csv.writer(f)
    writer.writerows(body)
    res = flask.make_response()
    res.data = f.getvalue()
    res.headers['Content-Type'] = 'text/csv'
    res.headers['Content-Disposition'] = 'attachment; filename=report.csv'
    return res


@app.route('/')
def get_index():
    user = get_login_user()
    events = []
    for event in get_events(lambda e: e["public_fg"]):
        events.append(sanitize_event(event))
    return flask.render_template('index.html', user=user, events=events, base_url=make_base_url(flask.request))


@app.route('/initialize')
def get_initialize():
    subprocess.call(["../../db/init.sh"])
    return ('', 204)


@app.route('/api/users', methods=['POST'])
def post_users():
    nickname = flask.request.json['nickname']
    login_name = flask.request.json['login_name']
    password = flask.request.json['password']

    conn = dbh()
    conn.autocommit(False)
    cur = conn.cursor()
    try:
        cur.execute("SELECT * FROM users WHERE login_name = %s", [login_name])
        duplicated = cur.fetchone()
        if duplicated:
            conn.rollback()
            return res_error('duplicated', 409)
        cur.execute(
            "INSERT INTO users (login_name, pass_hash, nickname) VALUES (%s, SHA2(%s, 256), %s)",
            [login_name, password, nickname])
        user_id = cur.lastrowid
        conn.commit()
    except MySQLdb.Error as e:
        conn.rollback()
        print(e)
        return res_error()
    return (jsonify({"id": user_id, "nickname": nickname}), 201)


@app.route('/api/users/<int:user_id>')
@login_required
def get_users(user_id):
    cur = dbh().cursor()
    cur.execute('SELECT id, nickname FROM users WHERE id = %s', [user_id])
    user = cur.fetchone()
    if user['id'] != get_login_user()['id']:
        return ('', 403)

    cur.execute(
        "SELECT r.*, s.rank AS sheet_rank, s.num AS sheet_num FROM reservations r INNER JOIN sheets s ON s.id = r.sheet_id WHERE r.user_id = %s ORDER BY IFNULL(r.canceled_at, r.reserved_at) DESC LIMIT 5",
        [user['id']])
    recent_reservations = []
    for row in cur.fetchall():
        event = get_event(row['event_id'])
        price = event['sheets'][row['sheet_rank']]['price']
        del event['sheets']
        del event['total']
        del event['remains']

        if row['canceled_at']:
            canceled_at = int(row['canceled_at'].replace(tzinfo=timezone.utc).timestamp())
        else:
            canceled_at = None

        recent_reservations.append({
            "id": int(row['id']),
            "event": event,
            "sheet_rank": row['sheet_rank'],
            "sheet_num": int(row['sheet_num']),
            "price": int(price),
            "reserved_at": int(row['reserved_at'].replace(tzinfo=timezone.utc).timestamp()),
            "canceled_at": canceled_at,
        })

    user['recent_reservations'] = recent_reservations
    cur.execute(
        "SELECT IFNULL(SUM(e.price + s.price), 0) AS total_price FROM reservations r INNER JOIN sheets s ON s.id = r.sheet_id INNER JOIN events e ON e.id = r.event_id WHERE r.user_id = %s AND r.canceled_at IS NULL",
        [user['id']])
    row = cur.fetchone()
    user['total_price'] = int(row['total_price'])

    cur.execute(
        "SELECT event_id FROM reservations WHERE user_id = %s GROUP BY event_id ORDER BY MAX(IFNULL(canceled_at, reserved_at)) DESC LIMIT 5",
        [user['id']])
    rows = cur.fetchall()
    recent_events = []
    for row in rows:
        event = get_event(row['event_id'])
        for sheet in event['sheets'].values():
            del sheet['detail']
        recent_events.append(event)
    user['recent_events'] = recent_events

    return jsonify(user)


@app.route('/api/actions/login', methods=['POST'])
def post_login():
    login_name = flask.request.json['login_name']
    password = flask.request.json['password']

    cur = dbh().cursor()

    cur.execute('SELECT * FROM users WHERE login_name = %s', [login_name])
    user = cur.fetchone()
    cur.execute('SELECT SHA2(%s, 256) AS pass_hash', [password])
    pass_hash = cur.fetchone()
    if not user or pass_hash['pass_hash'] != user['pass_hash']:
        return res_error("authentication_failed", 401)

    flask.session['user_id'] = user["id"]
    user = get_login_user()
    return flask.jsonify(user)


@app.route('/api/actions/logout', methods=['POST'])
@login_required
def post_logout():
    flask.session.pop('user_id', None)
    return ('', 204)


@app.route('/api/events')
def get_events_api():
    events = []
    for event in get_events(lambda e: e["public_fg"]):
        events.append(sanitize_event(event))
    return jsonify(events)


@app.route('/api/events/<int:event_id>')
def get_events_by_id(event_id):
    user = get_login_user()
    if user: event = get_event(event_id, user['id'])
    else: event = get_event(event_id)

    if not event or not event["public"]:
        return res_error("not_found", 404)

    event = sanitize_event(event)
    return jsonify(event)


@app.route('/api/events/<int:event_id>/actions/reserve', methods=['POST'])
@login_required
def post_reserve(event_id):
    rank = flask.request.json["sheet_rank"]

    user = get_login_user()
    event = get_event(event_id, user['id'])

    if not event or not event['public']:
        return res_error("invalid_event", 404)
    if not validate_rank(rank):
        return res_error("invalid_rank", 400)

    sheet = None
    reservation_id = 0

    while True:
        conn =  dbh()
        cur = conn.cursor()
        cur.execute(
            "SELECT * FROM sheets WHERE id NOT IN (SELECT sheet_id FROM reservations WHERE event_id = %s AND canceled_at IS NULL FOR UPDATE) AND `rank` =%s ORDER BY RAND() LIMIT 1",
            [event['id'], rank])
        sheet = cur.fetchone()
        if not sheet:
            return res_error("sold_out", 409)
        try:
            conn.autocommit(False)
            cur = conn.cursor()
            cur.execute(
                "INSERT INTO reservations (event_id, sheet_id, user_id, reserved_at) VALUES (%s, %s, %s, %s)",
                [event['id'], sheet['id'], user['id'], datetime.utcnow().strftime("%F %T.%f")])
            reservation_id = cur.lastrowid
            conn.commit()
        except MySQLdb.Error as e:
            conn.rollback()
            print(e)
        break

    content = jsonify({
        "id": reservation_id,
        "sheet_rank": rank,
        "sheet_num": sheet['num']})
    return flask.Response(content, status=202, mimetype='application/json')


@app.route('/api/events/<int:event_id>/sheets/<rank>/<int:num>/reservation', methods=['DELETE'])
@login_required
def delete_reserve(event_id, rank, num):
    user = get_login_user()
    event = get_event(event_id, user['id'])

    if not event or not event['public']:
        return res_error("invalid_event", 404)
    if not validate_rank(rank):
        return res_error("invalid_rank", 404)

    cur = dbh().cursor()
    cur.execute('SELECT * FROM sheets WHERE `rank` = %s AND num = %s', [rank, num])
    sheet = cur.fetchone()
    if not sheet:
        return res_error("invalid_sheet", 404)

    try:
        conn = dbh()
        conn.autocommit(False)
        cur = conn.cursor()

        cur.execute(
            "SELECT * FROM reservations WHERE event_id = %s AND sheet_id = %s AND canceled_at IS NULL GROUP BY event_id HAVING reserved_at = MIN(reserved_at) FOR UPDATE",
            [event['id'], sheet['id']])
        reservation = cur.fetchone()

        if not reservation:
            conn.rollback()
            return res_error("not_reserved", 400)
        if reservation['user_id'] != user['id']:
            conn.rollback()
            return res_error("not_permitted", 403)

        cur.execute(
            "UPDATE reservations SET canceled_at = %s WHERE id = %s",
            [datetime.utcnow().strftime("%F %T.%f"), reservation['id']])
        conn.commit()
    except MySQLdb.Error as e:
        conn.rollback()
        print(e)
        return res_error()

    return flask.Response(status=204)


@app.route('/admin/')
def get_admin():
    administrator = get_login_administrator()
    if administrator: events=get_events()
    else: events={}
    return flask.render_template('admin.html', administrator=administrator, events=events, base_url=make_base_url(flask.request))


@app.route('/admin/api/actions/login', methods=['POST'])
def post_adin_login():
    login_name = flask.request.json['login_name']
    password = flask.request.json['password']

    cur = dbh().cursor()

    cur.execute('SELECT * FROM administrators WHERE login_name = %s', [login_name])
    administrator = cur.fetchone()
    cur.execute('SELECT SHA2(%s, 256) AS pass_hash', [password])
    pass_hash = cur.fetchone()

    if not administrator or pass_hash['pass_hash'] != administrator['pass_hash']:
        return res_error("authentication_failed", 401)

    flask.session['administrator_id'] = administrator['id']
    administrator = get_login_administrator()
    return jsonify(administrator)


@app.route('/admin/api/actions/logout', methods=['POST'])
@admin_login_required
def get_admin_logout():
    flask.session.pop('administrator_id', None)
    return ('', 204)


@app.route('/admin/api/events')
@admin_login_required
def get_admin_events_api():
    return jsonify(get_events())


@app.route('/admin/api/events', methods=['POST'])
@admin_login_required
def post_admin_events_api():
    title = flask.request.json['title']
    public = flask.request.json['public']
    price = flask.request.json['price']

    conn = dbh()
    conn.autocommit(False)
    cur = conn.cursor()
    try:
        cur.execute(
            "INSERT INTO events (title, public_fg, closed_fg, price) VALUES (%s, %s, 0, %s)",
            [title, public, price])
        event_id = cur.lastrowid
        conn.commit()
    except MySQLdb.Error as e:
        conn.rollback()
        print(e)
    return jsonify(get_event(event_id))


@app.route('/admin/api/events/<int:event_id>')
@admin_login_required
def get_admin_events_by_id(event_id):
    event = get_event(event_id)
    if not event:
        return res_error("not_found", 404)
    return jsonify(event)


@app.route('/admin/api/events/<int:event_id>/actions/edit', methods=['POST'])
@admin_login_required
def post_event_edit(event_id):
    public = flask.request.json['public'] if 'public' in flask.request.json.keys() else False
    closed = flask.request.json['closed'] if 'closed' in flask.request.json.keys() else False
    if closed: public = False

    event = get_event(event_id)
    if not event:
        return res_error("not_found", 404)

    if event['closed']:
        return res_error('cannot_edit_closed_event', 400)
    elif event['public'] and closed:
        return res_error('cannot_close_public_event', 400)

    conn = dbh()
    conn.autocommit(False)
    cur = conn.cursor()
    try:
        cur.execute(
            "UPDATE events SET public_fg = %s, closed_fg = %s WHERE id = %s",
            [public, closed, event['id']])
        conn.commit()
    except MySQLdb.Error as e:
        conn.rollback()
    return jsonify(get_event(event_id))


@app.route('/admin/api/reports/events/<int:event_id>/sales')
@admin_login_required
def get_admin_event_sales(event_id):
    event = get_event(event_id)

    cur = dbh().cursor()
    reservations = cur.execute(
        'SELECT r.*, s.rank AS sheet_rank, s.num AS sheet_num, s.price AS sheet_price, e.price AS event_price FROM reservations r INNER JOIN sheets s ON s.id = r.sheet_id INNER JOIN events e ON e.id = r.event_id WHERE r.event_id = %s ORDER BY reserved_at ASC FOR UPDATE',
        [event['id']])
    reservations = cur.fetchall()
    reports = []

    for reservation in reservations:
        if reservation['canceled_at']:
            canceled_at = reservation['canceled_at'].isoformat()+"Z"
        else: canceled_at = ''
        reports.append({
            "reservation_id": reservation['id'],
            "event_id":       event['id'],
            "rank":           reservation['sheet_rank'],
            "num":            reservation['sheet_num'],
            "user_id":        reservation['user_id'],
            "sold_at":        reservation['reserved_at'].isoformat()+"Z",
            "canceled_at":    canceled_at,
            "price":          reservation['event_price'] + reservation['sheet_price'],
        })

    return render_report_csv(reports)


@app.route('/admin/api/reports/sales')
@admin_login_required
def get_admin_sales():
    cur = dbh().cursor()
    reservations = cur.execute('SELECT r.*, s.rank AS sheet_rank, s.num AS sheet_num, s.price AS sheet_price, e.id AS event_id, e.price AS event_price FROM reservations r INNER JOIN sheets s ON s.id = r.sheet_id INNER JOIN events e ON e.id = r.event_id ORDER BY reserved_at ASC FOR UPDATE')
    reservations = cur.fetchall()

    reports = []
    for reservation in reservations:
        if reservation['canceled_at']:
            canceled_at = reservation['canceled_at'].isoformat()+"Z"
        else: canceled_at = ''
        reports.append({
            "reservation_id": reservation['id'],
            "event_id":       reservation['event_id'],
            "rank":           reservation['sheet_rank'],
            "num":            reservation['sheet_num'],
            "user_id":        reservation['user_id'],
            "sold_at":        reservation['reserved_at'].isoformat()+"Z",
            "canceled_at":    canceled_at,
            "price":          reservation['event_price'] + reservation['sheet_price'],
        })
    return render_report_csv(reports)


if __name__ == "__main__":
    app.run(port=8080, debug=True, threaded=True)
