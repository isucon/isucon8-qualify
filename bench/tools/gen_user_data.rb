require 'forgery_ja'

if ARGV.count != 1 then
  puts "Usage: $0 count"
  exit 1
end

max = ARGV[0].to_i

ForgeryJa.dictionaries.reset!
ForgeryJa.load_paths << __dir__

m = {}
while m.keys.count != max do
  first_name = ForgeryJa(:name).first_name(to: ForgeryJa::ARRAY)
  last_name = ForgeryJa(:name).last_name(to: ForgeryJa::ARRAY)

  roma = last_name[ForgeryJa::ROMA] + "_" + first_name[ForgeryJa::ROMA]
  full_name = last_name[ForgeryJa::KANJI] + " " + first_name[ForgeryJa::KANJI]
  m[roma] = full_name
  # puts m.keys.count
end

m.each do |roma, full_name|
  email = "#{roma}@example.com"
  print full_name, "\t", email
  puts
end
