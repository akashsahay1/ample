export namespace api {
	
	export class Database {
	    name: string;
	    tables: number;
	    sizeBytes: number;
	
	    static createFrom(source: any = {}) {
	        return new Database(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.name = source["name"];
	        this.tables = source["tables"];
	        this.sizeBytes = source["sizeBytes"];
	    }
	}
	export class Extension {
	    name: string;
	    enabled: boolean;
	
	    static createFrom(source: any = {}) {
	        return new Extension(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.name = source["name"];
	        this.enabled = source["enabled"];
	    }
	}
	export class ExternalEnv {
	    kind: string;
	    name: string;
	    path: string;
	    running: boolean;
	    ports: number[];
	    php: string;
	    onPath: boolean;
	    canImport: boolean;
	    canStop: boolean;
	    sites: number;
	    databases: boolean;
	    notes: string[];
	
	    static createFrom(source: any = {}) {
	        return new ExternalEnv(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.kind = source["kind"];
	        this.name = source["name"];
	        this.path = source["path"];
	        this.running = source["running"];
	        this.ports = source["ports"];
	        this.php = source["php"];
	        this.onPath = source["onPath"];
	        this.canImport = source["canImport"];
	        this.canStop = source["canStop"];
	        this.sites = source["sites"];
	        this.databases = source["databases"];
	        this.notes = source["notes"];
	    }
	}
	export class ImportDatabase {
	    name: string;
	    sizeBytes: number;
	    exists: boolean;
	
	    static createFrom(source: any = {}) {
	        return new ImportDatabase(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.name = source["name"];
	        this.sizeBytes = source["sizeBytes"];
	        this.exists = source["exists"];
	    }
	}
	export class ImportSite {
	    name: string;
	    domain: string;
	    path: string;
	    docRoot: string;
	    php: string;
	    secure: boolean;
	    source: string;
	    conflict: string;
	
	    static createFrom(source: any = {}) {
	        return new ImportSite(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.name = source["name"];
	        this.domain = source["domain"];
	        this.path = source["path"];
	        this.docRoot = source["docRoot"];
	        this.php = source["php"];
	        this.secure = source["secure"];
	        this.source = source["source"];
	        this.conflict = source["conflict"];
	    }
	}
	export class ImportPlan {
	    kind: string;
	    source: string;
	    parkedDirs: string[];
	    sites: ImportSite[];
	    databases: ImportDatabase[];
	    missingPhp: string[];
	    notes: string[];
	
	    static createFrom(source: any = {}) {
	        return new ImportPlan(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.kind = source["kind"];
	        this.source = source["source"];
	        this.parkedDirs = source["parkedDirs"];
	        this.sites = this.convertValues(source["sites"], ImportSite);
	        this.databases = this.convertValues(source["databases"], ImportDatabase);
	        this.missingPhp = source["missingPhp"];
	        this.notes = source["notes"];
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	export class MySQLSource {
	    host: string;
	    port: number;
	    user: string;
	    password: string;
	
	    static createFrom(source: any = {}) {
	        return new MySQLSource(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.host = source["host"];
	        this.port = source["port"];
	        this.user = source["user"];
	        this.password = source["password"];
	    }
	}
	export class ImportRequest {
	    kind: string;
	    parkDirs: string[];
	    sites: string[];
	    databases: string[];
	    overwrite: boolean;
	    installPhp: boolean;
	    keepSecure: boolean;
	    mysql?: MySQLSource;
	
	    static createFrom(source: any = {}) {
	        return new ImportRequest(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.kind = source["kind"];
	        this.parkDirs = source["parkDirs"];
	        this.sites = source["sites"];
	        this.databases = source["databases"];
	        this.overwrite = source["overwrite"];
	        this.installPhp = source["installPhp"];
	        this.keepSecure = source["keepSecure"];
	        this.mysql = this.convertValues(source["mysql"], MySQLSource);
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	
	export class MySQLInfo {
	    running: boolean;
	    version: string;
	    host: string;
	    port: number;
	    user: string;
	    password: string;
	    dataDir: string;
	
	    static createFrom(source: any = {}) {
	        return new MySQLInfo(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.running = source["running"];
	        this.version = source["version"];
	        this.host = source["host"];
	        this.port = source["port"];
	        this.user = source["user"];
	        this.password = source["password"];
	        this.dataDir = source["dataDir"];
	    }
	}
	
	export class NewProjectRequest {
	    name: string;
	    kind: string;
	    directory: string;
	    php: string;
	    createDb: boolean;
	    database: string;
	
	    static createFrom(source: any = {}) {
	        return new NewProjectRequest(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.name = source["name"];
	        this.kind = source["kind"];
	        this.directory = source["directory"];
	        this.php = source["php"];
	        this.createDb = source["createDb"];
	        this.database = source["database"];
	    }
	}
	export class ServiceStatus {
	    name: string;
	    running: boolean;
	    pid: number;
	    version: string;
	    ports: number[];
	    error: string;
	
	    static createFrom(source: any = {}) {
	        return new ServiceStatus(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.name = source["name"];
	        this.running = source["running"];
	        this.pid = source["pid"];
	        this.version = source["version"];
	        this.ports = source["ports"];
	        this.error = source["error"];
	    }
	}
	export class Overview {
	    services: ServiceStatus[];
	    defaultPhp: string;
	    phpVersions: string[];
	    siteCount: number;
	    home: string;
	    appVersion: string;
	    caTrusted: boolean;
	
	    static createFrom(source: any = {}) {
	        return new Overview(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.services = this.convertValues(source["services"], ServiceStatus);
	        this.defaultPhp = source["defaultPhp"];
	        this.phpVersions = source["phpVersions"];
	        this.siteCount = source["siteCount"];
	        this.home = source["home"];
	        this.appVersion = source["appVersion"];
	        this.caTrusted = source["caTrusted"];
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	export class PHPSettings {
	    version: string;
	    ini: Record<string, string>;
	    extensions: Extension[];
	
	    static createFrom(source: any = {}) {
	        return new PHPSettings(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.version = source["version"];
	        this.ini = source["ini"];
	        this.extensions = this.convertValues(source["extensions"], Extension);
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	export class PHPVersion {
	    version: string;
	    full: string;
	    installed: boolean;
	    default: boolean;
	    eol: boolean;
	    siteCount: number;
	    downloadSize: number;
	
	    static createFrom(source: any = {}) {
	        return new PHPVersion(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.version = source["version"];
	        this.full = source["full"];
	        this.installed = source["installed"];
	        this.default = source["default"];
	        this.eol = source["eol"];
	        this.siteCount = source["siteCount"];
	        this.downloadSize = source["downloadSize"];
	    }
	}
	export class PortConflict {
	    port: number;
	    service: string;
	    process: string;
	    path: string;
	    env: string;
	
	    static createFrom(source: any = {}) {
	        return new PortConflict(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.port = source["port"];
	        this.service = source["service"];
	        this.process = source["process"];
	        this.path = source["path"];
	        this.env = source["env"];
	    }
	}
	
	export class Settings {
	    tld: string;
	    httpPort: number;
	    httpsPort: number;
	    mysqlPort: number;
	    parked: string[];
	    startServicesOnLaunch: boolean;
	    stopServicesOnQuit: boolean;
	    launchAtLogin: boolean;
	    home: string;
	
	    static createFrom(source: any = {}) {
	        return new Settings(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.tld = source["tld"];
	        this.httpPort = source["httpPort"];
	        this.httpsPort = source["httpsPort"];
	        this.mysqlPort = source["mysqlPort"];
	        this.parked = source["parked"];
	        this.startServicesOnLaunch = source["startServicesOnLaunch"];
	        this.stopServicesOnQuit = source["stopServicesOnQuit"];
	        this.launchAtLogin = source["launchAtLogin"];
	        this.home = source["home"];
	    }
	}
	export class Site {
	    name: string;
	    domain: string;
	    url: string;
	    path: string;
	    docRoot: string;
	    php: string;
	    isolated: boolean;
	    secure: boolean;
	    linked: boolean;
	    framework: string;
	
	    static createFrom(source: any = {}) {
	        return new Site(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.name = source["name"];
	        this.domain = source["domain"];
	        this.url = source["url"];
	        this.path = source["path"];
	        this.docRoot = source["docRoot"];
	        this.php = source["php"];
	        this.isolated = source["isolated"];
	        this.secure = source["secure"];
	        this.linked = source["linked"];
	        this.framework = source["framework"];
	    }
	}

}

