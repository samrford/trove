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

def export_env_line(key, fallback=''):
    # Build a single `KEY="value"` fragment for the serve_cmd.
    return key + '="' + envvar(key, fallback) + '" \\\n    '

local_resource(
  'backend',
  cmd='cd backend && go build -o bin/server ./cmd/server',
  serve_cmd='''
    cd backend && \\
    ''' +
    export_env_line('DATABASE_URL', 'postgres://trove:password@localhost:5434/trove?sslmode=disable') +
    export_env_line('SUPABASE_URL') +
    export_env_line('PORT', '8082') +
    export_env_line('STORAGE_ENDPOINT') +
    export_env_line('STORAGE_ACCESS_KEY_ID') +
    export_env_line('STORAGE_SECRET_ACCESS_KEY') +
    export_env_line('STORAGE_BUCKET') +
    export_env_line('STORAGE_REGION') +
    export_env_line('STORAGE_USE_PATH_STYLE') +
    export_env_line('GOOGLE_CLIENT_ID') +
    export_env_line('GOOGLE_CLIENT_SECRET') +
    export_env_line('GOOGLE_REDIRECT_URL') +
    export_env_line('GOOGLE_TOKEN_ENCRYPTION_KEY') +
    export_env_line('FRONTEND_ORIGIN') +
    '''./bin/server
  ''',
  deps=['backend', 'backend/.env'],
  ignore=['backend/bin']
)

local_resource(
  'frontend',
  cmd='cd frontend && bun install',
  serve_cmd='cd frontend && bun run dev',
  deps=['frontend/package.json'],
  ignore=['frontend/node_modules', 'frontend/build', 'frontend/.cache', 'frontend/.svelte-kit']
)
