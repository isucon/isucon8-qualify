require 'sinatra/base'

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

    get '/' do
      'Hello torb.ruby'
    end
  end
end
