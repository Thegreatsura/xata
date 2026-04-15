package policy

// Check memory limits and requests are equal for all containers
#Container: {
	resources?: {
		requests?: {
			memory?: string
			...
		}
		limits?: {
			memory?: string
			...
		}
		if requests.memory != _|_ && limits.memory != _|_ {
			// Ensure limits.memory equals requests.memory. Otherwise the
			// container might be OOM killed.
			limits: memory: requests.memory
		}
		...
	}
	...
}

// Match any object that has containers in its pod spec
#PodSpec: {
	containers?: [...#Container]
	initContainers?: [...#Container]
	...
}

// Any object that might have a pod spec
#K8sObject: {
	kind?: string
	metadata?: {
		name?: string
		...
	}
	spec?: {
		template?: {
			spec?: #PodSpec
			...
		}
		jobTemplate?: {
			spec?: {
				template?: {
					spec?: #PodSpec
					...
				}
				...
			}
			...
		}
		...
	}
	...
}

// When using -l 'kind' -l 'metadata.name', the data is structured as:
// [Kind]: [Name]: #K8sObject
[string]: [string]: #K8sObject
