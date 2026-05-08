docker_compose('docker-compose.yml')

# Load backend/.env (gitignored) into a dict. Values there override shell env,
# so you can drop a file at backend/.env and never think about it again.
def load_dotenv(path):
    result = {}
    contents = str(read_file(path, default=''))
    for line in contents.splitlines():
        line = line.strip()
        if not line or line.startswith('#'):
            continue
        if '=' not in line:
            continue
        k, v = line.split('=', 1)
        v = v.strip()
        if len(v) >= 2 and ((v[0] == '"' and v[-1] == '"') or (v[0] == "'" and v[-1] == "'")):
            v = v[1:-1]
        result[k.strip()] = v
    return result

env = load_dotenv('backend/.env')

def envvar(key, fallback=''):
    return env.get(key, os.environ.get(key, fallback))

local_resource(
  'backend',
  cmd='cd backend && go build -o bin/server ./cmd/server',
  serve_cmd='''
    cd backend && \
    DATABASE_URL="postgres://trove:password@localhost:5434/trove?sslmode=disable" \
    SUPABASE_URL="''' + envvar('SUPABASE_URL') + '''" \
    PORT="8082" \
    ./bin/server
  ''',
  deps=['backend'],
  ignore=['backend/bin']
)

# Frontend resource — uncomment after running `npx create-remix@latest .` inside frontend/
# local_resource(
#   'frontend',
#   cmd='cd frontend && npm install',
#   serve_cmd='cd frontend && npm run dev',
#   deps=['frontend/package.json'],
#   ignore=['frontend/node_modules', 'frontend/build', 'frontend/.cache', 'frontend/public/build']
# )
