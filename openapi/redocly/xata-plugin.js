function hasAtLeastOneXataScope() {
    return {
        Operation: {
            leave(operation, { report, location }) {
                const security = operation.security;
                const hasScope =
                    Array.isArray(security) &&
                    security.some(sec => Array.isArray(sec.xata) && sec.xata.length > 0);

                if (!hasScope) {
                    report({
                        message: 'Each operation must include at least one xata scope in its security declaration.',
                        location: location.child('security'),
                    });
                }
            },
        },
    };
}

const RESERVED_PATH_PARAMS = ['organizationID', 'projectID', 'branchID'];

function verifyReservedPathParameters() {
    return {
        PathItem(node, ctx) {
            for (const reserved of RESERVED_PATH_PARAMS) {
                for (const [method, operation] of Object.entries(node)) {
                    if (!['get', 'post', 'put', 'delete', 'patch'].includes(method.toLowerCase())) continue;
                    const params = [...(node.parameters || []), ...(operation.parameters || [])].map(param =>
                        param.$ref ? ctx.resolve(param).node : param
                    );
                    const found = params.some(p => p.in === 'path' && p.name === reserved);
                    if (ctx.key.includes(reserved) && !found) {
                        ctx.report({
                            message: `Path parameter '${reserved}' is reserved and must be defined in the path item parameters.`,
                        });
                    }
                }
            }
        }
    };
}

const OPERATION_KINDS = ['read', 'write', 'destructive'];

// x-operation-kind says what calling an operation does. The MCP server serves each
// kind from a separate tool and reads nothing but this label, so every operation
// must carry one: an HTTP method cannot tell a POST that queries apart from one
// that wipes data, and a missing label leaves the operation gated as destructive.
function verifyOperationKind() {
    return {
        Operation: {
            leave(operation, { report, location, key }) {
                const kind = operation['x-operation-kind'];
                const method = String(key).toLowerCase();

                if (kind === undefined) {
                    report({
                        message: `x-operation-kind is required; use one of: ${OPERATION_KINDS.join(', ')}.`,
                    });
                    return;
                }
                if (!OPERATION_KINDS.includes(kind)) {
                    report({
                        message: `x-operation-kind must be one of: ${OPERATION_KINDS.join(', ')}.`,
                        location: location.child('x-operation-kind'),
                    });
                    return;
                }
                if (method === 'delete' && kind !== 'destructive') {
                    report({
                        message: 'a DELETE operation must be x-operation-kind: destructive.',
                        location: location.child('x-operation-kind'),
                    });
                }
            },
        },
    };
}

function addPublicServers() {
    return {
        Root: {
            leave(root) {
                root.servers = [
                    { url: "https://api.xata.tech", description: "Xata API" },
                ];
            },
        },
        PathItem: {
            leave(pathItem) {
                if (Array.isArray(pathItem.servers) && pathItem.servers.length === 0) {
                    delete pathItem.servers;
                }
            },
        },
    };
}

export default function () {
    return {
        id: "xata",
        rules: {
            oas3: {
                "has-at-least-one-xata-scope": hasAtLeastOneXataScope,
                "verify-reserved-path-parameters": verifyReservedPathParameters,
                "verify-operation-kind": verifyOperationKind
            }
        },
        decorators: {
            oas3: {
                "add-public-servers": addPublicServers
            }
        }
    };
}
