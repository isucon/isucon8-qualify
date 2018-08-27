class Torb
  def call(env)
    [200, {'Content-Type' => 'text/html'}, ['Hello torb.ruby']]
  end
end

run Torb.new
