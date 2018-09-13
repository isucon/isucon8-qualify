import MySQLdb.cursors
import flask
import functools
import os
import pathlib
import copy
import json
from io import StringIO
import csv


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


if not os.path.exists(str(icons_folder)):
    os.makedirs(str(icons_folder))


def jsonify(target):
    return json.dumps(target, sort_keys=False, indent=2)


def login_required(f):
    @functools.wraps(f)
    def wrapper(*args, **kwargs):
        if not get_login_user():
            return ('', 401)
        return f(*args, **kwargs)
    return wrapper


def admin_login_required(f):
    @functools.wraps(f)
    def wrapper(*args, **kwargs):
        if not get_login_administrator():
            return ('', 401)
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


def get_events():
    conn = dbh()
    conn.autocommit(False)
    cur = conn.cursor()
    try:
        cur.execute("SELECT * FROM events ORDER BY id ASC")
        rows = cur.fetchall()
        event_ids = [row['id'] for row in rows]
        events = []
        for event_id in event_ids:
            event = get_event(event_id)
            events.append(event)
        conn.commit()
    except MySQLdb.Error as e:
        conn.rollback()
        raise e
    return events


def get_event(event_id, login_user_id=None):
    cur = dbh().cursor()
    cur.execute("SELECT * FROM events WHERE id = %s", [str(event_id)])
    event = cur.fetchone()
    # if not event: return event

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
            sheet['reserve'] = True
            sheet['reserved_at'] = reservation['reserved_at'].strftime('%s')
        else:
            event['remains'] += 1
            event['sheets'][sheet['rank']]['remains'] += 1

        event['sheets'][sheet['rank']]['detail'].append(sheet)

        #del sheet['id']
        #del sheet['price']
        #del sheet['rank']

    event['public'] = event['public_fg']
    event['closed'] = event['closed_fg']
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
    return cur.fetchone()


def body_params():
    pass


def render_report_csv(reports):
    reports = sorted(reports, key=lambda x:x['sold_at'])

    body = []
    body.append([",".join(["reservation_id", "event_id", "rank", "num", "price", "user_id", "sold_at", "canceled_at"]) + "\n"])
    for report in reports:
        values_list = list(report.values())
        body.append([",".join(map(str, values_list)) + "\n"])

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
    events = jsonify(get_events())  # TODO: sanitzed
    return flask.render_template('index.html', user=user, events=events)


@app.route('/initialize')
def get_initialize():
    cur = dbh().cursor()
    cur.execute("DELETE FROM user WHERE id > 1000")
    cur.execute("DELETE FROM reservations WHERE id > 1000")
    cur.execute("UPDATE reservations SET canceled_at = NULL")
    cur.execute("DELETE FROM events WHERE id > 3")
    cur.execute("UPDATE events SET public_fg = 0, closed_fg = 1")
    cur.execute("UPDATE events SET public_fg = 1, closed_fg = 0 WHERE id = 1")
    cur.execute("UPDATE events SET public_fg = 1, closed_fg = 0 WHERE id = 2")
    cur.execute("UPDATE events SET public_fg = 0, closed_fg = 0 WHERE id = 3")
    cur.close()
    return ('', 204)


@app.route('/api/users', methods=['POST'])
def post_users():
    nickname = flask.request.form['nickname']
    login_name = flask.request.form['login_name']
    password = flask.request.form['password']

    conn = dbh()
    conn.autocommit(False)
    cur = conn.cursor()
    try:
        cur.execute("DSELECT * FROM users WHERE login_name = %s", [login_name])
        duplicated = cur.fetchone()
        if duplicated:
            conn.rollback()
            return ('', 409)
        cur.execute(
            "INSERT INTO users (login_name, pass_hash, nickname) VALUES (%s, SHA2(%s, 256), %s)",
            [login_name, password, nickname])
        user_id = cur.lastrowid
        conn.commit()
    except MySQLdb.Error as e:
        conn.rollback()
        return ('', 500)
    return jsonify({id: user_id, nickname: nickname})


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
            canceled_at = row['canceled_at'].strftime('%s')
        else:
            canceled_at = None

        recent_reservations.append({
            "id": row['id'],
            "event": event,
            "sheet_rank": row['sheet_rank'],
            "sheet_num": row['sheet_num'],
            "price": price,
            "reserved_at": row['reserved_at'].strftime('%s'),
            "canceled_at": canceled_at,
        })

    user['recent_reservations'] = recent_reservations
    cur.execute(
        "SELECT IFNULL(SUM(e.price + s.price), 0) AS total_price FROM reservations r INNER JOIN sheets s ON s.id = r.sheet_id INNER JOIN events e ON e.id = r.event_id WHERE r.user_id = %s AND r.canceled_at IS NULL",
        [user['id']])
    row = cur.fetchone()
    user['total_price'] = int(row['total_price'])

    cur.execute(
        "SELECT DISTINCT event_id FROM reservations WHERE user_id = %s ORDER BY IFNULL(canceled_at, reserved_at) DESC LIMIT 5",
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
    login_name = flask.request.form['login_name']
    password = flask.request.form['password']

    cur = dbh().cursor()

    cur.execute('SELECT  * FROM users WHERE login_name = %s', [login_name])
    user = cur.fetchone()
    cur.execute('SELECT SHA2(?, 256) AS pass_hash', [password])
    pass_hash = cur.fetchone()
    if not user or pass_hash['pass_hash'] != user['pass_hash']: flask.abort(401)

    flask.session['user_id'] = user["id"]
    user = get_login_user()
    return flask.jsonify(user)


@app.route('/api/actions/logout')
def get_logout():
    flask.session.pop('user_id', None)
    return ('', 204)


@app.route('/api/events')
def get_events_api():
    events = []
    for event in get_events():
        sanitize_event(event)
    return jsonify(events)


@app.route('/api/events/<int:event_id>')
def get_events_by_id(event_id):
    user = get_login_user() or {}
    event = get_event(event_id, user['id'])
    if not event or not event["public"]: flask.abort(404)

    event = sanitize_event(event)
    return jsonify(event)


@app.route('/api/events/:id/actions/reserve', methods=['POST'])
def post_reserve():
    pass


@app.route('/admin')
def get_admin():
    administrator = get_login_administrator()
    if administrator:
        return flask.render_template('admin.html', events=get_events())


@app.route('/admin/api/actions/login', methods=['POST'])
def post_adin_login():
    login_name = flask.request.form['login_name']
    password = flask.request.form['password']

    cur = dbh().cursor()

    cur.execute('SELECT * FROM administrators WHERE login_name = %s', [login_name])
    administrator = cur.fetchone()
    cur.execute('SELECT SHA2(?, 256) AS pass_hash', [password])
    pass_hash = cur.fetchone()

    if not administrator or pass_hash['pass_hash'] !=  administrator['pass_hash']:
        return ('', 401)

    flask.session['administrator_id'] = administrator['id']
    administrator = get_login_administrator()
    return jsonify(administrator)


@app.route('/admin/api/actions/logout')
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
    title = flask.request.form['title']
    public = flask.request.form['public']
    price = flask.request.form['price']

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
    return jsonify(get_event(event_id))


@app.route('/admin/api/events/<int:event_id>')
@admin_login_required
def get_admin_events_by_id(event_id):
    event = get_event(event_id)
    if not event: flask.abort(404)
    return jsonify(event)


@app.route('/admin/api/events/<int:event_id>actions/edit', methods=['POST'])
@admin_login_required
def post_event_edit(event_id):
    public = flask.request.form['public']
    closed = flask.request.form['closed']
    if closed: public = False

    event = get_event(event_id)
    if not event: flask.abort(404)

    if event['closed']:
        flask.abort(400)
    elif event['public'] and closed:
        flask.abort(400)

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
            canceled_at = reservation['canceled_at'].isoformat()
        else: canceled_at = ''
        reports.append({
            "reservation_id": reservation['id'],
            "event_id":       event['id'],
            "rank":           reservation['sheet_rank'],
            "num":            reservation['sheet_num'],
            "user_id":        reservation['user_id'],
            "sold_at":        reservation['reserved_at'].isoformat(),
            "canceled_at":    canceled_at,
            "price":          reservation['event_price'] + reservation['sheet_price'],
        })

    return render_report_csv(reports)


@app.route('/admin/api/reports/sales')
@admin_login_required
def get_admin_sales(event_id):
    cur = dbh().cursor()
    reservations = cur.execute('SELECT r.*, s.rank AS sheet_rank, s.num AS sheet_num, s.price AS sheet_price, e.id AS event_id, e.price AS event_price FROM reservations r INNER JOIN sheets s ON s.id = r.sheet_id INNER JOIN events e ON e.id = r.event_id ORDER BY reserved_at ASC FOR UPDATE')
    reservations = cur.fetchall()

    reports = []
    for reservation in reservations:
        if reservation['canceled_at']:
            canceled_at = reservation['canceled_at'].isoformat()
        else: canceled_at = ''
        reports.append({
            "reservation_id": reservation['id'],
            "event_id":       reservation['event_id'],
            "rank":           reservation['sheet_rank'],
            "num":            reservation['sheet_num'],
            "user_id":        reservation['user_id'],
            "sold_at":        reservation['reserved_at'].isoformat(),
            "canceled_at":    canceled_at,
            "price":          reservation['event_price'] + reservation['sheet_price'],
        })
    return render_report_csv(reports)


if __name__ == "__main__":
    app.run(port=5000, debug=True, threaded=True)
