require 'json'
require 'sinatra/base'
require 'erubi'
require 'mysql2'
require 'mysql2-cs-bind'

module Torb
  class Web < Sinatra::Base
    configure :development do
      require 'sinatra/reloader'
      register Sinatra::Reloader
    end

    set :root, File.expand_path('../..', __dir__)
    set :public_folder, File.expand_path('../static', root)
    set :sessions, key: 'torb_session', expire_after: 3600
    set :session_secret, 'tagomoris'

    set :erb, escape_html: true

    set :login_required, ->(value) do
      condition do
        if value && !get_login_user
          halt_with_error 401, 'login_required'
        end
      end
    end

    before '/api/*' do
      content_type :json
    end

    helpers do
      def db
        Thread.current[:db] ||= Mysql2::Client.new(
          host: ENV['DB_HOST'],
          port: ENV['DB_PORT'],
          username: ENV['DB_USER'],
          password: ENV['DB_PASS'],
          database: ENV['DB_DATABASE'],
          database_timezone: :utc,
          reconnect: true,
          init_command: 'SET SESSION sql_mode="STRICT_TRANS_TABLES,NO_ZERO_IN_DATE,NO_ZERO_DATE,ERROR_FOR_DIVISION_BY_ZERO,NO_ENGINE_SUBSTITUTION"',
        )
      end

      def get_events(where = nil)
        where ||= ->(e) { e['public_fg'] == 1 }

        db.query('BEGIN')
        event_ids = db.query('SELECT * FROM events ORDER BY id ASC').select(&where).map { |e| e['id'] }
        events = event_ids.map do |event_id|
          event = get_event(event_id)
          event['sheets'].each { |sheet| sheet.delete('detail') }
          event
        end
        db.query('COMMIT')

        events
      end

      def get_event(event_id, login_user_id = nil)
        event = db.xquery('SELECT * FROM events WHERE id = ?', event_id).first
        return unless event

        # zero fill
        event['total']   = 0
        event['remains'] = 0
        event['sheets'] = {}
        %w[S A B C].each do |rank|
          event['sheets'][rank] = { 'total' => 0, 'remains' => 0, 'detail' => [] }
        end

        sheets = db.query('SELECT * FROM sheets ORDER BY `rank`, num')
        sheets.each do |sheet|
          event['sheets'][sheet['rank']]['price'] ||= event['price'] + sheet['price']
          event['total'] += 1
          event['sheets'][sheet['rank']]['total'] += 1

          reservation = db.xquery('SELECT * FROM reservations WHERE event_id = ? AND sheet_id = ? AND canceled_at IS NULL GROUP BY event_id, sheet_id HAVING reserved_at = MIN(reserved_at)', event['id'], sheet['id']).first
          if reservation
            sheet['mine']        = true if login_user_id && reservation['user_id'] == login_user_id
            sheet['reserved']    = true
            sheet['reserved_at'] = reservation['reserved_at'].to_i
          else
            event['remains'] += 1
            event['sheets'][sheet['rank']]['remains'] += 1
          end

          event['sheets'][sheet['rank']]['detail'].push(sheet)

          sheet.delete('id')
          sheet.delete('price')
          sheet.delete('rank')
        end

        event['public'] = (event.delete('public_fg') == 1)
        event['closed'] = (event.delete('closed_fg') == 1)

        event
      end

      def sanitize_event(event)
        sanitized = event.dup  # shallow clone
        sanitized.delete('price')
        sanitized.delete('public')
        sanitized.delete('closed')
        sanitized
      end

      def get_login_user
        user_id = session[:user_id]
        return unless user_id
        db.xquery('SELECT id, nickname FROM users WHERE id = ?', user_id).first
      end

      def validate_rank(rank)
        db.xquery('SELECT COUNT(*) AS total_sheets FROM sheets WHERE `rank` = ?', rank).first['total_sheets'] > 0
      end

      def body_params
        @body_params ||= JSON.parse(request.body.tap(&:rewind).read)
      end

      def halt_with_error(status = 500, error = 'unknown')
        halt status, { error: error }.to_json
      end
    end

    get '/' do
      @user   = get_login_user
      @events = get_events.map(&method(:sanitize_event))
      erb :index
    end

    post '/api/users' do
      nickname   = body_params['nickname']
      login_name = body_params['login_name']
      password   = body_params['password']

      db.query('BEGIN')
      begin
        duplicated = db.xquery('SELECT * FROM users WHERE login_name = ?', login_name).first
        if duplicated
          db.query('ROLLBACK')
          halt_with_error 409, 'duplicated'
        end

        db.xquery('INSERT INTO users (login_name, pass_hash, nickname) VALUES (?, SHA2(?, 256), ?)', login_name, password, nickname)
        user_id = db.last_id
        db.query('COMMIT')
      rescue => e
        warn "rollback by: #{e}"
        db.query('ROLLBACK')
        halt_with_error
      end

      status 201
      { id: user_id, nickname: nickname }.to_json
    end

    get '/api/users/:id', login_required: true do |user_id|
      user = db.xquery('SELECT id, nickname FROM users WHERE id = ?', user_id).first
      if user['id'] != get_login_user['id']
        halt_with_error 403, 'forbidden'
      end

      rows = db.xquery('SELECT r.*, s.rank AS sheet_rank, s.num AS sheet_num FROM reservations r INNER JOIN sheets s ON s.id = r.sheet_id WHERE r.user_id = ? ORDER BY IFNULL(r.canceled_at, r.reserved_at) DESC LIMIT 5', user['id'])
      recent_reservations = rows.map do |row|
        event = get_event(row['event_id'])
        price = event['sheets'][row['sheet_rank']]['price']
        event.delete('sheets')
        event.delete('total')
        event.delete('remains')

        {
          id:          row['id'],
          event:       event,
          sheet_rank:  row['sheet_rank'],
          sheet_num:   row['sheet_num'],
          price:       price,
          reserved_at: row['reserved_at'].to_i,
          canceled_at: row['canceled_at']&.to_i,
        }
      end

      user['recent_reservations'] = recent_reservations
      user['total_price'] = db.xquery('SELECT IFNULL(SUM(e.price + s.price), 0) AS total_price FROM reservations r INNER JOIN sheets s ON s.id = r.sheet_id INNER JOIN events e ON e.id = r.event_id WHERE r.user_id = ? AND r.canceled_at IS NULL', user['id']).first['total_price']

      rows = db.xquery('SELECT DISTINCT event_id FROM reservations WHERE user_id = ? ORDER BY IFNULL(canceled_at, reserved_at) DESC LIMIT 5', user['id'])
      recent_events = rows.map do |row|
        event = get_event(row['event_id'])
        event['sheets'].each { |_, sheet| sheet.delete('detail') }
        event
      end
      user['recent_events'] = recent_events

      user.to_json
    end


    post '/api/actions/login' do
      login_name = body_params['login_name']
      password   = body_params['password']

      user      = db.xquery('SELECT * FROM users WHERE login_name = ?', login_name).first
      pass_hash = db.xquery('SELECT SHA2(?, 256) AS pass_hash', password).first['pass_hash']
      halt_with_error 401, 'authentication_failed' if user.nil? || pass_hash != user['pass_hash']

      session['user_id'] = user['id']

      user = get_login_user
      user.to_json
    end

    post '/api/actions/logout', login_required: true do
      session.delete('user_id')
      status 204
    end

    get '/api/events' do
      events = get_events.map(&method(:sanitize_event))
      events.to_json
    end

    get '/api/events/:id' do |event_id|
      user = get_login_user || {}
      event = get_event(event_id, user['id'])
      halt_with_error 404, 'not_found' if event.nil? || !event['public']

      event = sanitize_event(event)
      event.to_json
    end

    post '/api/events/:id/actions/reserve', login_required: true do |event_id|
      rank = body_params['sheet_rank']

      user  = get_login_user
      event = get_event(event_id, user['id'])
      halt_with_error 404, 'invalid_event' unless event && event['public']
      halt_with_error 400, 'invalid_rank' unless validate_rank(rank)

      sheet = nil
      reservation_id = nil
      loop do
        sheet = db.xquery('SELECT * FROM sheets WHERE id NOT IN (SELECT sheet_id FROM reservations WHERE event_id = ? AND canceled_at IS NULL FOR UPDATE) AND `rank` = ? ORDER BY RAND() LIMIT 1', event['id'], rank).first
        halt_with_error 409, 'sold_out' unless sheet
        db.query('BEGIN')
        begin
          db.xquery('INSERT INTO reservations (event_id, sheet_id, user_id, reserved_at) VALUES (?, ?, ?, ?)', event['id'], sheet['id'], user['id'], Time.now.utc.strftime('%F %T.%6N'))
          reservation_id = db.last_id
          db.query('COMMIT')
        rescue => e
          db.query('ROLLBACK')
          warn "re-try: rollback by #{e}"
          next
        end

        break
      end

      status 202
      { reservation_id: reservation_id, sheet_rank: rank, sheet_num: sheet['num'] } .to_json
    end

    delete '/api/events/:id/sheets/:rank/:num/reservation', login_required: true do |event_id, rank, num|
      user  = get_login_user
      event = get_event(event_id, user['id'])
      halt_with_error 404, 'invalid_event' unless event && event['public']
      halt_with_error 404, 'invalid_rank'  unless validate_rank(rank)

      sheet = db.xquery('SELECT * FROM sheets WHERE `rank` = ? AND num = ?', rank, num).first
      halt_with_error 404, 'invalid_sheet' unless sheet

      db.query('BEGIN')
      begin
        reservation = db.xquery('SELECT * FROM reservations WHERE event_id = ? AND sheet_id = ? AND canceled_at IS NULL GROUP BY event_id HAVING reserved_at = MIN(reserved_at) FOR UPDATE', event['id'], sheet['id']).first
        unless reservation
          db.query('ROLLBACK')
          halt_with_error 400, 'not_reserved'
        end
        if reservation['user_id'] != user['id']
          db.query('ROLLBACK')
          halt_with_error 403, 'not_permitted'
        end

        db.xquery('UPDATE reservations SET canceled_at = ? WHERE id = ?', Time.now.utc.strftime('%F %T.%6N'), reservation['id'])
        db.query('COMMIT')
      rescue => e
        warn "rollback by: #{e}"
        db.query('ROLLBACK')
        halt_with_error
      end

      status 204
    end
  end
end
